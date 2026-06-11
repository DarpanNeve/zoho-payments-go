package zoho

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAmountUnmarshal(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    float64
		wantErr bool
	}{
		{"decimal string", `"100.50"`, 100.50, false},
		{"integer string", `"4999"`, 4999, false},
		{"number", `100.5`, 100.5, false},
		{"integer number", `250`, 250, false},
		{"empty string", `""`, 0, false},
		{"null", `null`, 0, false},
		{"garbage", `"abc"`, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a Amount
			err := json.Unmarshal([]byte(tc.in), &a)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && a.Float64() != tc.want {
				t.Fatalf("got %v, want %v", a.Float64(), tc.want)
			}
		})
	}
}

func TestAmountMarshal(t *testing.T) {
	b, err := json.Marshal(Amount(1499.5))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "1499.5" {
		t.Fatalf("got %s", b)
	}
}

func TestTimeUnmarshal(t *testing.T) {
	epoch := time.Date(2026, 6, 1, 10, 30, 0, 0, time.UTC)
	ms := epoch.UnixMilli()

	cases := []struct {
		name    string
		in      string
		want    time.Time
		wantErr bool
	}{
		{"epoch ms number", `1780309800000`, time.UnixMilli(1780309800000), false},
		{"epoch ms string", `"1780309800000"`, time.UnixMilli(1780309800000), false},
		{"rfc3339", `"2026-06-01T10:30:00Z"`, epoch, false},
		{"null", `null`, time.Time{}, false},
		{"zero", `0`, time.Time{}, false},
		{"empty", `""`, time.Time{}, false},
		{"garbage", `"not-a-time"`, time.Time{}, true},
	}
	_ = ms
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ts Time
			err := json.Unmarshal([]byte(tc.in), &ts)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && !ts.Time.Equal(tc.want) {
				t.Fatalf("got %v, want %v", ts.Time, tc.want)
			}
		})
	}
}

func TestCheckRespFlexibleCode(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		status  int
		wantErr bool
	}{
		{"success numeric zero", `{"code":0,"message":"ok"}`, 200, false},
		{"success no code", `{"payment":{}}`, 200, false},
		{"error numeric", `{"code":9004,"message":"invalid amount"}`, 400, true},
		{"error string code", `{"code":"error","message":"bad request"}`, 400, true},
		{"error string code http 200", `{"code":"error","message":"soft failure"}`, 200, true},
		{"nonzero code http 200", `{"code":57,"message":"limit"}`, 200, true},
		{"http error empty body", ``, 500, true},
		{"http error non-json", `gateway timeout`, 504, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkResp([]byte(tc.body), tc.status)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	got := SanitizeText("Trek <Booking> & Stay; -- (2 pax) + extras!!")
	want := "Trek Booking  Stay -- (2 pax) + extras"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizePhone(t *testing.T) {
	if got := NormalizePhone("919876543210", "IN"); got != "9876543210" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizePhone("9876543210", "IN"); got != "9876543210" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizePhone("14155550123", "US"); got != "14155550123" {
		t.Fatalf("got %q", got)
	}
}
