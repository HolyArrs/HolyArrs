package updates

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
)

// Update list settings in the this_app group.
const (
	settingListSerial   = "update_list_serial"    // the highest accepted serial
	settingListFailures = "update_list_failures"  // server failures in a row
	settingListBucket   = "update_rollout_bucket" // this install's rollout number, 0-99, made once
)

// listFailuresForCopy is how many server failures in a row make the check
// also read the Pages copy.
const listFailuresForCopy = 3

// signatureProblem is shown when the update list's signature fails.
const signatureProblem = "We could not check the update info, so we are ignoring it. Your app is fine. We will try again later."

func (e *Checker) listKeys() []PublicKey {
	if e.ListKeys != nil {
		return e.ListKeys
	}
	return Pinned()
}

func (e *Checker) listURL() string {
	if e.ListURL != "" {
		return e.ListURL
	}
	return DefaultURL
}

func (e *Checker) listCopyURL() string {
	if e.ListCopyURL != "" {
		return e.ListCopyURL
	}
	return PagesURL
}

// listMode says the signed update list is used: a key is pinned and a list
// address is set. Otherwise the GitHub releases check runs as before.
func (e *Checker) listMode() bool {
	return len(e.listKeys()) > 0 && e.listURL() != ""
}

// bucket is this install's rollout number, made once and kept.
func (e *Checker) bucket(ctx context.Context) int {
	if n, err := strconv.Atoi(e.setting(ctx, settingListBucket)); err == nil && n >= 0 && n < 100 {
		return n
	}
	r, err := rand.Int(rand.Reader, big.NewInt(100))
	if err != nil {
		return 0
	}
	n := int(r.Int64())
	_ = e.saveSetting(ctx, settingListBucket, strconv.Itoa(n))
	return n
}

// readList reads the signed update list from the server, and from the
// Pages copy after listFailuresForCopy failures or a bad signature; when
// both answer, the higher serial wins.
func (e *Checker) readList(ctx context.Context) (latest, link, notes string, none bool, err error) {
	keys := e.listKeys()
	minSerial, _ := strconv.ParseInt(e.setting(ctx, settingListSerial), 10, 64)
	fails, _ := strconv.Atoi(e.setting(ctx, settingListFailures))

	best, serr := e.fetchList(ctx, e.listURL(), keys, minSerial)
	// Once the server has failed often, the Pages copy is read too (until
	// the server answers again), so a frozen server loses to a newer copy.
	tryCopy := fails >= listFailuresForCopy || errors.Is(serr, ErrSignature)
	if serr != nil {
		fails++
	} else {
		fails = 0
	}
	_ = e.saveSetting(ctx, settingListFailures, strconv.Itoa(fails))
	if cp := e.listCopyURL(); cp != "" && (tryCopy || fails >= listFailuresForCopy) {
		if l, cerr := e.fetchList(ctx, cp, keys, minSerial); cerr == nil && (best == nil || l.Serial > best.Serial) {
			best = l
		}
	}
	if best == nil {
		if errors.Is(serr, ErrSignature) {
			return "", "", "", false, errors.New(signatureProblem)
		}
		return "", "", "", false, serr
	}
	if best.Serial > minSerial {
		_ = e.saveSetting(ctx, settingListSerial, strconv.FormatInt(best.Serial, 10))
	}
	b := e.bucket(ctx)
	c, ok := best.Offer("stable", b)
	if e.setting(ctx, settingUpdatePre) == "true" {
		if beta, bok := best.Offer("beta", b); bok && (!ok || Newer(beta.Version, c.Version)) {
			c, ok = beta, true
		}
	}
	if !ok {
		return "", "", "", true, nil
	}
	return c.Version, c.ChangelogURL, c.NotesShort, false, nil
}

func (e *Checker) fetchList(ctx context.Context, link string, keys []PublicKey, minSerial int64) (*List, error) {
	data, err := e.fetch(ctx, link)
	if err != nil {
		return nil, err
	}
	sig, err := e.fetch(ctx, link+SigSuffix)
	if err != nil {
		return nil, err
	}
	return Parse(data, sig, keys, e.now(), minSerial)
}

func (e *Checker) fetch(ctx context.Context, link string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, errors.New("the update list address is not a link")
	}
	req.Header.Set("User-Agent", "HolyArrs/"+e.Version)
	c := *e.client(ctx)
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		if r.URL.Scheme != via[0].URL.Scheme {
			return errors.New("redirect changes the scheme")
		}
		return nil
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.New("the update list did not answer")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the update list answered %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, updateMaxBody+1))
	if err != nil || len(b) > updateMaxBody {
		return nil, errors.New("the update list could not be read")
	}
	return b, nil
}
