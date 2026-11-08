package rowtpl

import (
	"reflect"
	"testing"
	"time"
)

func TestSeasonOrder_ActiveSoonRest(t *testing.T) {
	cases := []struct {
		date, region string
		want         []string
	}{
		{"2026-10-24", "us", []string{"halloween", "dia-de-muertos", "fall", "thanksgiving", "christmas", "new-year", "lunar-new-year", "valentines", "spring", "pride", "summer", "back-to-school"}},
		{"2026-12-10", "us", []string{"fall", "christmas", "new-year", "lunar-new-year", "valentines", "spring", "pride", "summer", "back-to-school", "halloween", "dia-de-muertos", "thanksgiving"}},
		{"2027-02-05", "us", []string{"lunar-new-year", "valentines", "spring", "pride", "summer", "back-to-school", "fall", "halloween", "dia-de-muertos", "thanksgiving", "christmas", "new-year"}},
		{"2027-07-01", "au", []string{"spring", "halloween", "dia-de-muertos", "thanksgiving", "christmas", "summer", "new-year", "lunar-new-year", "back-to-school", "valentines", "fall", "pride"}},
	}
	for _, c := range cases {
		var got []string
		for _, s := range SeasonOrder(day(c.date), c.region) {
			got = append(got, s.Key)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s:\n got  %v\n want %v", c.date, c.region, got, c.want)
		}
	}
}

func TestSeasonActive_Windows(t *testing.T) {
	if !SeasonActive("halloween", day("2026-10-24"), "us") || SeasonActive("christmas", day("2026-10-24"), "us") {
		t.Fatal("Oct 24: Halloween on, Christmas off")
	}
	if !SeasonActive("thanksgiving", day("2026-10-05"), "ca") || SeasonActive("thanksgiving", day("2026-10-05"), "us") {
		t.Fatal("Canada's window is early October")
	}
	if !SeasonActive("summer", day("2027-01-10"), "au") || SeasonActive("summer", day("2027-01-10"), "us") {
		t.Fatal("south summer is December to March")
	}
}

func TestLunarDates_CoverTwoFutureYears(t *testing.T) {
	n := 0
	for _, d := range lunarDates {
		if d.After(time.Now()) {
			n++
		}
	}
	if n < 2 {
		t.Fatalf("lunar table has %d future dates; extend it", n)
	}
}

// day parses a YYYY-MM-DD test date as UTC.
func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}
