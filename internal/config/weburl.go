package config

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// The web of this installation: where the person picks a plan and calls a
// machine from the phone. One binary serves every installation, so links in
// texts are built from it, never written for the production site.

// prodPaymentPage — the production plan page; billing campaigns count on
// this address.
const prodPaymentPage = "https://chat.execai.ru/settings/payment"

// WebBase — the address of the web chat. The one the sign-in was confirmed
// on wins (saved); otherwise it is derived from api_base: a trailing /api is
// cut ("https://chat.example.org/api" → "https://chat.example.org"), or the
// "api." host prefix ("https://api.execai.ru" → "https://execai.ru").
func WebBase(apiBase, saved string) string {
	if s := strings.TrimRight(strings.TrimSpace(saved), "/"); s != "" {
		return s
	}
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return base
	}
	if p := strings.TrimRight(u.Path, "/"); strings.HasSuffix(p, "/api") {
		u.Path = strings.TrimSuffix(p, "/api")
	} else if strings.HasPrefix(u.Host, "api.") {
		u.Host = strings.TrimPrefix(u.Host, "api.")
	}
	return strings.TrimRight(u.String(), "/")
}

// PaymentURL — the page to pick a plan on, on the web of this installation,
// with the agent's campaign marks. A sign-in without a saved web base on the
// production API keeps the production page.
func PaymentURL(apiBase, savedWebBase, campaign string) string {
	q := "?utm_source=agent&utm_medium=cli&utm_campaign=" + url.QueryEscape(campaign)
	if strings.TrimSpace(savedWebBase) == "" &&
		strings.TrimRight(strings.TrimSpace(apiBase), "/") == strings.TrimRight(Defaults().APIBase, "/") {
		return prodPaymentPage + q
	}
	base := WebBase(apiBase, savedWebBase)
	if base == "" {
		return prodPaymentPage + q
	}
	return base + "/settings/payment" + q
}

// Installation — the API base (config.json) and the saved web base
// (credentials.json) of this installation, read without writing anything:
// missing files give the defaults.
func Installation() (apiBase, savedWebBase string) {
	apiBase = Defaults().APIBase
	dir, err := Dir()
	if err != nil {
		return apiBase, ""
	}
	var c struct {
		APIBase string `json:"api_base"`
	}
	if data, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil && json.Unmarshal(data, &c) == nil &&
		strings.TrimSpace(c.APIBase) != "" {
		apiBase = c.APIBase
	}
	if cr, err := LoadCredentials(); err == nil && cr != nil {
		savedWebBase = cr.WebBase
	}
	return apiBase, savedWebBase
}

// PaymentURLHere — PaymentURL for this installation.
func PaymentURLHere(campaign string) string {
	api, web := Installation()
	return PaymentURL(api, web, campaign)
}

// WebBaseHere — WebBase for this installation.
func WebBaseHere() string {
	api, web := Installation()
	return WebBase(api, web)
}
