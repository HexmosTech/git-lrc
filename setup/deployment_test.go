package setup

import "testing"

func TestIsCloudAPIURL(t *testing.T) {
	tests := []struct {
		apiURL string
		want   bool
	}{
		{apiURL: "", want: true},
		{apiURL: CloudAPIURL, want: true},
		{apiURL: CloudAPIURL + "/", want: true},
		{apiURL: "http://localhost:8888", want: false},
		{apiURL: "https://review.acme.corp", want: false},
	}

	for _, tc := range tests {
		if got := IsCloudAPIURL(tc.apiURL); got != tc.want {
			t.Fatalf("IsCloudAPIURL(%q) = %v, want %v", tc.apiURL, got, tc.want)
		}
	}
}
