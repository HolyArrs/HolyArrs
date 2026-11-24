// Command holyarrs-pluginkey makes and uses the plugin signing key. It runs
// offline on the maintainer's own PC and never prints private key material.
//
//	pluginkey keygen -out <dir>
//	pluginkey keygen -plain -pipe -pub <file>
//	pluginkey sign -key <file> -kind plugins|blocklist|languages -in <payload.json> -out <list.json> [-min-counter N]
//	pluginkey verify -pub <file> -in <list.json>
//	pluginkey check -pub <file> -list <plugins.json> -blocklist <blocklist.json> -manifest <manifest.json> -file <plugin>
//
// A key is plain or sealed. A plain key is the base64 ed25519 seed on one
// line, with no passphrase: the release automation keeps it as a GitHub
// environment secret and signs with it unattended. "keygen -plain -pipe"
// writes a new plain key only to stdout, for piping straight into
// "gh secret set", and refuses when stdout is a terminal.
//
// A sealed key holds the seed sealed with XChaCha20-Poly1305 under a key
// derived from a passphrase with scrypt (golang.org/x/crypto). keygen and sign
// read that passphrase from the terminal, or with -pass-env NAME from that
// environment variable. sign needs no passphrase for a plain key.
package main

import (
	"bytes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/term"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

const (
	keyFile = "plugin-signing.key"
	pubFile = "plugin-signing.pub"
	// scrypt cost; stored in the key file so it can rise later.
	scryptN = 1 << 17
	scryptR = 8
	scryptP = 1
)

var errPassphrase = errors.New("wrong passphrase or damaged key file")

// sealedKey is the private key file.
type sealedKey struct {
	KDF        string `json:"kdf"`
	N          int    `json:"n"`
	R          int    `json:"r"`
	P          int    `json:"p"`
	Salt       []byte `json:"salt"`
	Cipher     string `json:"cipher"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "keygen":
		err = runKeygen(os.Args[2:])
	case "sign":
		err = runSign(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "check":
		err = runCheck(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: pluginkey keygen|sign|verify|check [flags]")
	os.Exit(2)
}

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "folder for plugin-signing.key and plugin-signing.pub")
	passEnv := fs.String("pass-env", "", "read the passphrase from this environment variable")
	plain := fs.Bool("plain", false, "make a key with no passphrase")
	pipe := fs.Bool("pipe", false, "with -plain: write the private key only to stdout")
	pubOut := fs.String("pub", "", "with -pipe: public key file to write")
	_ = fs.Parse(args)
	if *plain {
		if !*pipe || *pubOut == "" {
			return errors.New("-plain needs -pipe and -pub")
		}
		if term.IsTerminal(int(os.Stdout.Fd())) {
			return errors.New("-pipe writes the private key to stdout; pipe it into a secret store, not a terminal")
		}
		pub, err := keygenPlain(os.Stdout, *pubOut)
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Public key: ", base64.StdEncoding.EncodeToString(pub))
		fmt.Fprintln(os.Stderr, "Fingerprint:", fingerprint(pub))
		return nil
	}
	if *out == "" {
		return errors.New("-out is required")
	}
	pass, err := readPassphrase("New passphrase: ", *passEnv)
	if err != nil {
		return err
	}
	again := pass
	if *passEnv == "" {
		again, err = readPassphrase("Same passphrase again: ", "")
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(pass, again) {
		return errors.New("the passphrases differ")
	}
	if len(pass) < 12 {
		return errors.New("use a passphrase of at least 12 characters")
	}
	pub, err := keygen(*out, pass)
	if err != nil {
		return err
	}
	fmt.Println("Public key: ", base64.StdEncoding.EncodeToString(pub))
	fmt.Println("Fingerprint:", fingerprint(pub))
	return nil
}

func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	key := fs.String("key", "", "plugin-signing.key")
	kind := fs.String("kind", "", "plugins, blocklist or languages")
	in := fs.String("in", "", "payload JSON")
	out := fs.String("out", "", "signed list to write")
	minCounter := fs.Uint64("min-counter", 0, "the payload counter must be above this")
	passEnv := fs.String("pass-env", "", "sealed keys: read the passphrase from this environment variable")
	_ = fs.Parse(args)
	if *key == "" || *in == "" || *out == "" {
		return errors.New("-key, -in and -out are required")
	}
	var pass []byte
	if sealed, err := isSealed(*key); err != nil {
		return err
	} else if sealed {
		if pass, err = readPassphrase("Passphrase: ", *passEnv); err != nil {
			return err
		}
	}
	if err := sign(*key, pass, *kind, *in, *out, *minCounter); err != nil {
		return err
	}
	fmt.Println("Signed list written to", *out)
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pub := fs.String("pub", "", "plugin-signing.pub")
	in := fs.String("in", "", "signed list")
	_ = fs.Parse(args)
	if *pub == "" || *in == "" {
		return errors.New("-pub and -in are required")
	}
	l, err := verify(*pub, *in)
	if err != nil {
		return err
	}
	fmt.Printf("OK: %s list, counter %d, %d entries\n", l.Kind, l.Counter, len(l.Entries))
	return nil
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	pub := fs.String("pub", "", "plugin-signing.pub")
	list := fs.String("list", "", "signed plugins.json")
	block := fs.String("blocklist", "", "signed blocklist.json")
	man := fs.String("manifest", "", "manifest.json")
	file := fs.String("file", "", "plugin file")
	_ = fs.Parse(args)
	if *pub == "" || *list == "" || *block == "" || *man == "" || *file == "" {
		return errors.New("-pub, -list, -blocklist, -manifest and -file are required")
	}
	m, err := check(*pub, *list, *block, *man, *file)
	if err != nil {
		return err
	}
	fmt.Printf("OK: the app would install %s %s\n", m.ID, m.Version)
	return nil
}

// check runs the app's own install check on a published set of files.
func check(pubPath, listPath, blockPath, manPath, filePath string) (*plugins.Manifest, error) {
	pub, err := readPub(pubPath)
	if err != nil {
		return nil, err
	}
	var raw [3][]byte
	for i, p := range []string{listPath, blockPath, manPath} {
		if raw[i], err = os.ReadFile(p); err != nil {
			return nil, err
		}
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	acc, err := plugins.CheckInstall(plugins.InstallInput{PublicKey: pub, List: raw[0], Blocklist: raw[1],
		Manifest: raw[2], File: f, TempDir: os.TempDir()})
	if err != nil {
		return nil, err
	}
	defer acc.Close()
	return acc.Manifest, nil
}

// readPassphrase reads from the environment variable env when set, else
// from the terminal.
func readPassphrase(prompt, env string) ([]byte, error) {
	if env != "" {
		// A secret pasted or piped in may carry a line ending; a passphrase never does.
		p := strings.TrimRight(os.Getenv(env), "\r\n")
		if p == "" {
			return nil, fmt.Errorf("environment variable %s is empty", env)
		}
		return []byte(p), nil
	}
	fmt.Fprint(os.Stderr, prompt)
	p, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("reading passphrase (run this in a terminal): %w", err)
	}
	return p, nil
}

func fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "SHA256:" + hex.EncodeToString(sum[:])
}

// keygen writes a new key pair into dir and returns the public key.
func keygen(dir string, pass []byte) (ed25519.PublicKey, error) {
	keyPath, pubPath := filepath.Join(dir, keyFile), filepath.Join(dir, pubFile)
	for _, p := range []string{keyPath, pubPath} {
		if _, err := os.Stat(p); err == nil {
			return nil, fmt.Errorf("%s already exists; not overwriting", p)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sealed, err := seal(priv.Seed(), pass)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(sealed, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeNew(keyPath, data, 0o600); err != nil {
		return nil, err
	}
	if err := writeNew(pubPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return nil, err
	}
	return pub, nil
}

// keygenPlain writes a new plain private key to w and the public key to
// pubPath; the private key never touches the disk.
func keygenPlain(w io.Writer, pubPath string) (ed25519.PublicKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := writeNew(pubPath, []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintln(w, base64.StdEncoding.EncodeToString(priv.Seed())); err != nil {
		return nil, err
	}
	return pub, nil
}

// plainSeed returns the seed when data is a plain key.
func plainSeed(data []byte) ([]byte, bool) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	return seed, err == nil && len(seed) == ed25519.SeedSize
}

// isSealed reports whether the key file needs a passphrase.
func isSealed(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	_, plain := plainSeed(data)
	return !plain, nil
}

// writeNew creates path and fails if it already exists.
func writeNew(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func seal(seed, pass []byte) (*sealedKey, error) {
	s := &sealedKey{KDF: "scrypt", N: scryptN, R: scryptR, P: scryptP, Cipher: "xchacha20poly1305",
		Salt: make([]byte, 16), Nonce: make([]byte, chacha20poly1305.NonceSizeX)}
	if _, err := rand.Read(s.Salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(s.Nonce); err != nil {
		return nil, err
	}
	aead, err := s.aead(pass)
	if err != nil {
		return nil, err
	}
	s.Ciphertext = aead.Seal(nil, s.Nonce, seed, []byte(s.KDF+s.Cipher))
	return s, nil
}

func (s *sealedKey) aead(pass []byte) (cipher.AEAD, error) {
	k, err := scrypt.Key(pass, s.Salt, s.N, s.R, s.P, chacha20poly1305.KeySize)
	if err != nil {
		return nil, err
	}
	return chacha20poly1305.NewX(k)
}

// loadKey opens the private key file: a plain key as is, a sealed one with pass.
func loadKey(path string, pass []byte) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if seed, ok := plainSeed(data); ok {
		return ed25519.NewKeyFromSeed(seed), nil
	}
	var s sealedKey
	if err := json.Unmarshal(data, &s); err != nil || s.KDF != "scrypt" || s.Cipher != "xchacha20poly1305" ||
		len(s.Nonce) != chacha20poly1305.NonceSizeX {
		return nil, errors.New("not a plugin signing key file")
	}
	aead, err := s.aead(pass)
	if err != nil {
		return nil, err
	}
	seed, err := aead.Open(nil, s.Nonce, s.Ciphertext, []byte(s.KDF+s.Cipher))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errPassphrase
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// sign signs the payload in inPath and writes the signed list to outPath.
func sign(keyPath string, pass []byte, kind, inPath, outPath string, minCounter uint64) error {
	if kind != plugins.KindPlugins && kind != plugins.KindBlocklist && kind != plugins.KindLanguages {
		return errors.New("-kind must be plugins, blocklist or languages")
	}
	raw, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	var l plugins.List
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	if l.Kind != kind {
		return fmt.Errorf("payload kind is %q, not %q", l.Kind, kind)
	}
	if l.Counter <= minCounter {
		return fmt.Errorf("payload counter %d is not above %d", l.Counter, minCounter)
	}
	priv, err := loadKey(keyPath, pass)
	if err != nil {
		return err
	}
	out, err := plugins.SignList(priv, l)
	if err != nil {
		return err
	}
	// Check it the way the app will before writing it.
	if _, err := plugins.VerifyList(priv.Public().(ed25519.PublicKey), out, kind, 0); err != nil {
		return fmt.Errorf("the app would refuse this list: %w", err)
	}
	return writeNew(outPath, out, 0o644)
}

// verify checks a signed list against the public key file.
func verify(pubPath, inPath string) (*plugins.List, error) {
	pub, err := readPub(pubPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		return nil, err
	}
	var sl plugins.SignedList
	if err := json.Unmarshal(data, &sl); err != nil {
		return nil, fmt.Errorf("%w: %v", plugins.ErrListFormat, err)
	}
	var head struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(sl.Payload, &head)
	return plugins.VerifyList(pub, data, head.Kind, 0)
}

// readPub reads a public key file.
func readPub(path string) (ed25519.PublicKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("not a plugin signing public key")
	}
	return pub, nil
}
