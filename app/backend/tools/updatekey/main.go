// Command holyarrs-updatekey makes and uses the update list signing key. It
// runs offline on the maintainer's own PC (or in the release workflow) and
// never prints private key material.
//
//	updatekey keygen -out <dir>
//	updatekey keygen -plain -pipe -pub <file>
//	updatekey check -key <file> [-pass-env NAME]
//	updatekey sign -key <file> -in <file> -out <file.minisig> [-binary] [-pass-env NAME]
//	updatekey list -version V -dist <dir> -base <url> -out <manifest.json> [-auto-safe] [-one-way] [-security] [-notes TEXT]
//	updatekey verify -pub <file> -in <list.json> -sig <list.json.minisig>
//
// Signatures and the public key use minisign's format, which the app checks
// (internal/updates). The private key file holds the key id and the ed25519
// seed sealed with XChaCha20-Poly1305 under a scrypt key from the passphrase,
// or is a plain key (no passphrase, see plain.go): keygen -plain -pipe writes
// it only to stdout, for piping straight into "gh secret set", and refuses
// when stdout is a terminal. sign reads either kind.
package main

import (
	"bytes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/term"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

const (
	keyFile = "update-signing.key"
	pubFile = "update-signing.pub"
	scryptN = 1 << 17
	scryptR = 8
	scryptP = 1
)

var errPassphrase = errors.New("wrong passphrase or damaged key file")

// sealedKey is the private key file.
type sealedKey struct {
	ID         []byte `json:"id"`
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
	case "check":
		err = runCheck(os.Args[2:])
	case "sign":
		err = runSign(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	case "list":
		err = runList(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: updatekey keygen|check|sign|verify|list [flags]")
	os.Exit(2)
}

func runKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "folder for update-signing.key and update-signing.pub")
	plain := fs.Bool("plain", false, "make a key with no passphrase")
	pipe := fs.Bool("pipe", false, "with -plain: write the private key only to stdout")
	pubOut := fs.String("pub", "", "with -pipe: public key file to write")
	_ = fs.Parse(args)
	if *plain {
		if !*pipe {
			return errors.New("-plain needs -pipe and -pub")
		}
		return runKeygenPlain(*pubOut)
	}
	if *out == "" {
		return errors.New("-out is required")
	}
	pass, err := readPassphrase("New passphrase: ")
	if err != nil {
		return err
	}
	again, err := readPassphrase("Same passphrase again: ")
	if err != nil {
		return err
	}
	if !bytes.Equal(pass, again) {
		return errors.New("the passphrases differ")
	}
	if len(pass) < 12 {
		return errors.New("use a passphrase of at least 12 characters")
	}
	k, err := keygen(*out, pass)
	if err != nil {
		return err
	}
	printPublic(k)
	return nil
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	key := fs.String("key", "", "update-signing.key")
	passEnv := fs.String("pass-env", "", "read the passphrase from this environment variable")
	_ = fs.Parse(args)
	if *key == "" {
		return errors.New("-key is required")
	}
	pass, err := passphrase(*passEnv)
	if err != nil {
		return err
	}
	id, priv, err := loadKey(*key, pass)
	if err != nil {
		return err
	}
	fmt.Println("OK: the passphrase opens the key.")
	printPublic(publicOf(id, priv))
	return nil
}

func runSign(args []string) error {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)
	key := fs.String("key", "", "update-signing.key")
	in := fs.String("in", "", "update list JSON")
	out := fs.String("out", "", "signature file to write (list.json.minisig)")
	passEnv := fs.String("pass-env", "", "read the passphrase from this environment variable")
	binary := fs.Bool("binary", false, "sign a program, not a JSON list")
	_ = fs.Parse(args)
	if *key == "" || *in == "" || *out == "" {
		return errors.New("-key, -in and -out are required")
	}
	if err := signFile(*key, *passEnv, *in, *out, !*binary); err != nil {
		return err
	}
	fmt.Println("Signature written to", *out)
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	pub := fs.String("pub", "", "update-signing.pub")
	in := fs.String("in", "", "update list JSON")
	sig := fs.String("sig", "", "signature file")
	_ = fs.Parse(args)
	if *pub == "" || *in == "" || *sig == "" {
		return errors.New("-pub, -in and -sig are required")
	}
	trusted, err := verify(*pub, *in, *sig)
	if err != nil {
		return err
	}
	fmt.Println("OK:", trusted)
	return nil
}

// passphrase reads it from env when named (release workflow), else asks.
func passphrase(env string) ([]byte, error) {
	if env != "" {
		v := os.Getenv(env)
		if v == "" {
			return nil, fmt.Errorf("%s is empty", env)
		}
		return []byte(v), nil
	}
	return readPassphrase("Passphrase: ")
}

func readPassphrase(prompt string) ([]byte, error) {
	fmt.Fprint(os.Stderr, prompt)
	p, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("reading passphrase (run this in a terminal): %w", err)
	}
	return p, nil
}

func printPublic(k updates.PublicKey) {
	fmt.Println("Public key:", publicLine(k))
	fmt.Println("Key id:    ", k.KeyID())
}

func publicOf(id [8]byte, priv ed25519.PrivateKey) updates.PublicKey {
	return updates.PublicKey{ID: id, Key: priv.Public().(ed25519.PublicKey)}
}

// publicLine is the key in minisign's public key format (what PinnedKeys holds).
func publicLine(k updates.PublicKey) string {
	return base64.StdEncoding.EncodeToString(append(append([]byte("Ed"), k.ID[:]...), k.Key...))
}

// keygen writes a new key pair into dir and returns the public key.
func keygen(dir string, pass []byte) (updates.PublicKey, error) {
	var none updates.PublicKey
	keyPath, pubPath := filepath.Join(dir, keyFile), filepath.Join(dir, pubFile)
	for _, p := range []string{keyPath, pubPath} {
		if _, err := os.Stat(p); err == nil {
			return none, fmt.Errorf("%s already exists; not overwriting", p)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return none, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return none, err
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return none, err
	}
	sealed, err := seal(id, priv.Seed(), pass)
	if err != nil {
		return none, err
	}
	data, err := json.MarshalIndent(sealed, "", "  ")
	if err != nil {
		return none, err
	}
	if err := writeNew(keyPath, data, 0o600); err != nil {
		return none, err
	}
	k := publicOf(id, priv)
	pubText := "untrusted comment: HolyArrs update list key " + k.KeyID() + "\n" + publicLine(k) + "\n"
	if err := writeNew(pubPath, []byte(pubText), 0o644); err != nil {
		return none, err
	}
	return k, nil
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

func seal(id [8]byte, seed, pass []byte) (*sealedKey, error) {
	s := &sealedKey{ID: id[:], KDF: "scrypt", N: scryptN, R: scryptR, P: scryptP, Cipher: "xchacha20poly1305",
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
	s.Ciphertext = aead.Seal(nil, s.Nonce, seed, s.ad())
	return s, nil
}

func (s *sealedKey) ad() []byte { return append([]byte(s.KDF+s.Cipher), s.ID...) }

func (s *sealedKey) aead(pass []byte) (cipher.AEAD, error) {
	k, err := scrypt.Key(pass, s.Salt, s.N, s.R, s.P, chacha20poly1305.KeySize)
	if err != nil {
		return nil, err
	}
	return chacha20poly1305.NewX(k)
}

// loadKey opens the private key file with pass.
func loadKey(path string, pass []byte) ([8]byte, ed25519.PrivateKey, error) {
	var id [8]byte
	data, err := os.ReadFile(path)
	if err != nil {
		return id, nil, err
	}
	var s sealedKey
	if err := json.Unmarshal(data, &s); err != nil || s.KDF != "scrypt" || s.Cipher != "xchacha20poly1305" ||
		len(s.Nonce) != chacha20poly1305.NonceSizeX || len(s.ID) != 8 {
		return id, nil, errors.New("not an update signing key file")
	}
	aead, err := s.aead(pass)
	if err != nil {
		return id, nil, err
	}
	seed, err := aead.Open(nil, s.Nonce, s.Ciphertext, s.ad())
	if err != nil || len(seed) != ed25519.SeedSize {
		return id, nil, errPassphrase
	}
	copy(id[:], s.ID)
	return id, ed25519.NewKeyFromSeed(seed), nil
}

// sign writes a minisign "ED" signature of the list, checked the way the
// app will check it before it is written.
func sign(keyPath string, pass []byte, inPath, outPath string) error {
	id, priv, err := loadKey(keyPath, pass)
	if err != nil {
		return err
	}
	return signWith(id, priv, inPath, outPath, true)
}

// signFile signs with a plain key, or a sealed one opened with the passphrase.
func signFile(keyPath, passEnv, inPath, outPath string, isList bool) error {
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	if id, priv, ok := parsePlain(raw); ok {
		return signWith(id, priv, inPath, outPath, isList)
	}
	pass, err := passphrase(passEnv)
	if err != nil {
		return err
	}
	id, priv, err := loadKey(keyPath, pass)
	if err != nil {
		return err
	}
	return signWith(id, priv, inPath, outPath, isList)
}

func signWith(id [8]byte, priv ed25519.PrivateKey, inPath, outPath string, isList bool) error {
	data, err := os.ReadFile(inPath)
	if err != nil {
		return err
	}
	if isList && !json.Valid(data) {
		return errors.New("the update list is not valid JSON")
	}
	h := blake2b.Sum512(data)
	s := ed25519.Sign(priv, h[:])
	trusted := fmt.Sprintf("timestamp:%d\tfile:%s", time.Now().Unix(), filepath.Base(inPath))
	global := ed25519.Sign(priv, append(bytes.Clone(s), trusted...))
	raw := append(append([]byte("ED"), id[:]...), s...)
	pub := publicOf(id, priv)
	sig := "untrusted comment: signature from HolyArrs update key " + pub.KeyID() + "\n" +
		base64.StdEncoding.EncodeToString(raw) + "\n" +
		"trusted comment: " + trusted + "\n" +
		base64.StdEncoding.EncodeToString(global) + "\n"
	if _, err := updates.Verify(data, []byte(sig), []updates.PublicKey{pub}); err != nil {
		return fmt.Errorf("the app would refuse this signature: %w", err)
	}
	return writeNew(outPath, []byte(sig), 0o644)
}

// verify checks a list and its signature against the public key file.
func verify(pubPath, inPath, sigPath string) (string, error) {
	b, err := os.ReadFile(pubPath)
	if err != nil {
		return "", err
	}
	pub, err := updates.ParsePublicKey(string(b))
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(inPath)
	if err != nil {
		return "", err
	}
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		return "", err
	}
	return updates.Verify(data, sig, []updates.PublicKey{pub})
}
