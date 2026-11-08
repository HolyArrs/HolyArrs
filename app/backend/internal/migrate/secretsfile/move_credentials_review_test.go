package secretsfile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMoveAddressesCannotReflectSecrets(t *testing.T) {
	for _, address := range []string{
		"http://user:private@host/", "http://host/?apikey=private", "http://host/#private",
		"http://host/?", "ftp://host/", "http:///path",
	} {
		t.Run(address, func(t *testing.T) {
			for _, dry := range []bool{true, false} {
				d := deps(t)
				f := &File{Version: 1, Radarr: KeyApp{Address: address, APIKey: "your-test-key-private"}}
				lines := Apply(context.Background(), d, f, dry)
				if !strings.HasPrefix(results(lines)["radarr (old app)"], "failed:") {
					t.Fatalf("accepted: %+v", lines)
				}
				data, _ := json.Marshal(lines)
				if strings.Contains(string(data), "private") {
					t.Fatal("address secret reflected")
				}
				list, err := ListMoveCredentials(context.Background(), d.Store, d.Vault)
				if err != nil || len(list) != 0 {
					t.Fatalf("credential saved: %+v %v", list, err)
				}
			}
		})
	}
}
