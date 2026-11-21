package youtube

import (
	"errors"
	"testing"
)

func TestParseLink(t *testing.T) {
	const id = "dQw4w9WgXcQ"
	const watch = "https://www.youtube.com/watch?v=" + id
	const uc = "UCHnyfMqiRRG1u-2MsSQLbXA"
	ok := []struct {
		raw  string
		want Link
	}{
		{"https://www.youtube.com/watch?v=" + id, Link{KindVideo, id, watch}},
		{"  https://youtube.com/watch?v=" + id + "  ", Link{KindVideo, id, watch}},
		{"http://m.youtube.com/watch?v=" + id + "&t=10s", Link{KindVideo, id, watch}},
		{"https://youtu.be/" + id + "?t=3", Link{KindVideo, id, watch}},
		{"https://YouTu.be/" + id, Link{KindVideo, id, watch}},
		{"https://www.youtube.com/shorts/" + id, Link{KindVideo, id, watch}},
		{"https://www.youtube.com/live/" + id + "?feature=share", Link{KindVideo, id, watch}},
		{"https://music.youtube.com/watch?v=" + id, Link{KindVideo, id, watch}},
		{"https://www.youtube.com/watch?v=" + id + "&list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", Link{KindVideo, id, watch}},
		{"https://www.youtube.com/playlist?list=PLxxxxxxxx", Link{KindPlaylist, "PLxxxxxxxx", "https://www.youtube.com/playlist?list=PLxxxxxxxx"}},
		{"https://WWW.YOUTUBE.COM/playlist?list=PLxxxxxxxx", Link{KindPlaylist, "PLxxxxxxxx", "https://www.youtube.com/playlist?list=PLxxxxxxxx"}},
		{"https://www.youtube.com/@Veritasium", Link{KindChannel, "@Veritasium", "https://www.youtube.com/@Veritasium"}},
		{"https://www.youtube.com/@Veritasium/videos", Link{KindChannel, "@Veritasium", "https://www.youtube.com/@Veritasium"}},
		{"https://www.youtube.com/@Veritasium/", Link{KindChannel, "@Veritasium", "https://www.youtube.com/@Veritasium"}},
		{"https://www.youtube.com/channel/" + uc, Link{KindChannel, uc, "https://www.youtube.com/channel/" + uc}},
		{"https://www.youtube.com/channel/" + uc + "/streams", Link{KindChannel, uc, "https://www.youtube.com/channel/" + uc}},
		{"https://www.youtube.com/c/veritasium", Link{KindChannel, "c/veritasium", "https://www.youtube.com/c/veritasium"}},
		{"https://www.youtube.com/user/1veritasium", Link{KindChannel, "user/1veritasium", "https://www.youtube.com/user/1veritasium"}},
	}
	for _, c := range ok {
		got, err := ParseLink(c.raw)
		if err != nil || got != c.want {
			t.Errorf("ParseLink(%q) = %+v, %v; want %+v", c.raw, got, err, c.want)
		}
	}
	refused := []string{
		"",
		"   ",
		"https://youtube.com.evil.com/watch?v=" + id,
		"https://evil.com/watch?v=" + id,
		"https://youtube.com@evil.com/watch?v=" + id,
		"https://user@youtube.com/watch?v=" + id,
		"https://youtube.com:8080/watch?v=" + id,
		"javascript:alert(1)",
		"ftp://youtube.com/watch?v=" + id,
		"youtube.com/watch?v=" + id,
		"--exec=x",
		"https://www.youtube.com/watch?v=short",
		"https://www.youtube.com/watch?v=" + id + "%0A",
		"https://www.youtube.com/watch?v=--exec=xxxx",
		"https://www.youtube.com/watch",
		"https://www.youtube.com/feed/subscriptions",
		"https://www.youtube.com/results?search_query=x",
		"https://www.youtube.com/",
		"https://www.youtube.com/playlist?list=WL",
		"https://www.youtube.com/playlist?list=PL%20xxxxxxxxx",
		"https://www.youtube.com/channel/UCshort",
		"https://www.youtube.com/@ab",
		"https://www.youtube.com/@Veritasium/community/x",
		"https://www.youtube.com/@Veritasium/about",
		"https://www.youtube.com/c/",
		"https://www.youtube.com/user/a%2Fb",
		"https://youtu.be/" + id + "/x",
		"https://youtu.be/@Veritasium",
		"https://www.youtube.com/shorts/" + id + "x",
	}
	for _, raw := range refused {
		if got, err := ParseLink(raw); !errors.Is(err, ErrNotYouTube) {
			t.Errorf("ParseLink(%q) = %+v, %v; want ErrNotYouTube", raw, got, err)
		}
	}
}
