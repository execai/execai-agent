package sync

import (
	"encoding/json"
	"testing"

	"github.com/velesbsdllc/agent-vbai/internal/config"
)

// The agent reads config.json with encoding/json, which matches a field in
// any spelling of its name: "UI_BROWSER" is ui_browser to it. So are the
// settings that belong to one computer to sync — a filter that compared the
// name byte for byte let "Ui_Browser" travel, and with it the path of a
// program to start.
func TestMachineOnlyKeys_AnySpelling(t *testing.T) {
	read := func(data []byte) config.Config {
		t.Helper()
		var c config.Config
		if err := json.Unmarshal(data, &c); err != nil {
			t.Fatalf("%s: %v", data, err)
		}
		return c
	}

	// Arriving: none of the spellings reaches a computer that has no such
	// setting of its own.
	local := []byte(`{"api_base":"https://c.example"}`)
	for _, remote := range []string{
		`{"api_base":"https://x","selected_model_id":"m2","ui_browser":"/tmp/not-a-browser"}`,
		`{"api_base":"https://x","selected_model_id":"m2","UI_BROWSER":"/tmp/not-a-browser"}`,
		`{"api_base":"https://x","selected_model_id":"m2","Ui_Browser":"/tmp/not-a-browser"}`,
		`{"api_base":"https://x","selected_model_id":"m2","Notifications":"off","MODEL_AUTO":"m9","ui_Browser":"/tmp/x","UI_browser":"/tmp/y"}`,
	} {
		c := read(keepMachineOnly(local, []byte(remote)))
		if c.UIBrowser != "" || c.Notifications != "" || c.ModelAuto != "" {
			t.Errorf("arriving %s\n-> ui_browser %q, notifications %q, model_auto %q", remote, c.UIBrowser, c.Notifications, c.ModelAuto)
		}
		if c.SelectedModelID != "m2" {
			t.Errorf("arriving %s: the shared settings were lost: %+v", remote, c)
		}
	}

	// This computer's own value stays, however it and the arriving one are
	// spelled.
	for _, own := range []string{
		`{"api_base":"https://c.example","ui_browser":"/usr/bin/mine","notifications":"bell"}`,
		`{"api_base":"https://c.example","UI_Browser":"/usr/bin/mine","NOTIFICATIONS":"bell"}`,
	} {
		for _, remote := range []string{
			`{"api_base":"https://x","selected_model_id":"m2"}`,
			`{"api_base":"https://x","selected_model_id":"m2","ui_browser":"/tmp/other","notifications":"off"}`,
			`{"api_base":"https://x","selected_model_id":"m2","Ui_Browser":"/tmp/other","Notifications":"off"}`,
			`{"api_base":"https://x","selected_model_id":"m2","UI_BROWSER":"/tmp/other","zz_last":"1","Ui_browser":"/tmp/third"}`,
		} {
			c := read(keepMachineOnly([]byte(own), []byte(remote)))
			if c.UIBrowser != "/usr/bin/mine" || c.Notifications != "bell" || c.SelectedModelID != "m2" {
				t.Errorf("own %s, arriving %s\n-> %+v", own, remote, c)
			}
		}
	}

	// An arriving config that brings nothing of the kind, to a computer
	// that has nothing of the kind, is written as it came.
	plain := []byte(`{"api_base":"https://x","selected_model_id":"m2"}`)
	if got := keepMachineOnly(local, plain); string(got) != string(plain) {
		t.Errorf("an untouched config was rewritten: %s", got)
	}

	// Leaving: none of the spellings is packed for the server.
	for _, mine := range []string{
		`{"api_base":"https://a","selected_model_id":"m1","ui_browser":"/usr/bin/x","notifications":"off","model_auto":"m1"}`,
		`{"api_base":"https://a","selected_model_id":"m1","Ui_Browser":"/usr/bin/x","NOTIFICATIONS":"off","Model_Auto":"m1"}`,
	} {
		c := read(stripMachineOnly([]byte(mine)))
		if c.UIBrowser != "" || c.Notifications != "" || c.ModelAuto != "" || c.SelectedModelID != "m1" {
			t.Errorf("packed for the server from %s: %+v", mine, c)
		}
	}
	untouched := []byte(`{"api_base":"https://a","selected_model_id":"m1"}`)
	if got := stripMachineOnly(untouched); string(got) != string(untouched) {
		t.Errorf("a config without such settings was rewritten: %s", got)
	}

	// The same settings are the same to sync whether or not a computer's own
	// ones are there, in any spelling: changing only them is no change.
	want := settingHash("config.json", []byte(`{"api_base":"https://a","selected_model_id":"m1"}`))
	for _, data := range []string{
		`{"api_base":"https://b","selected_model_id":"m1","ui_browser":"/usr/bin/x"}`,
		`{"api_base":"https://b","selected_model_id":"m1","UI_BROWSER":"/usr/bin/y","Notifications":"off","MODEL_auto":"m1"}`,
	} {
		if got := settingHash("config.json", []byte(data)); got != want {
			t.Errorf("%s hashes as another setting", data)
		}
	}
}
