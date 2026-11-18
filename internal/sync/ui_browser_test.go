package sync

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ui_browser — the browser execai ui opens its window with — names a
// program of this computer: it never reaches the server, an arriving config
// leaves this machine's value alone (or its absence), and changing only it
// sends nothing.
func TestEngine_ConfigUIBrowserStaysLocal(t *testing.T) {
	stub := newStub(t)
	key := mustKey(t)
	a, b := newSide(t, stub, key), newSide(t, stub, key)
	a.write(a.eng.Paths.ConfigFile(), `{"api_base":"https://a.example","selected_model_id":"glm-5.3","ui_browser":"/usr/bin/google-chrome"}`, time.Time{})
	b.write(b.eng.Paths.ConfigFile(), `{"api_base":"https://b.example","ui_browser":"C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe"}`, past)
	a.run(Options{})

	bl := stub.blob(KindConfig, NameConfig)
	if bl == nil {
		t.Fatal("no config blob")
	}
	_, _, data, err := unseal(key, bl.content)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := unpackFiles(data)
	if strings.Contains(string(files["config.json"]), "ui_browser") || strings.Contains(string(files["config.json"]), "google-chrome") ||
		!strings.Contains(string(files["config.json"]), "glm-5.3") {
		t.Fatalf("config.json on the server: %s", files["config.json"])
	}

	b.run(Options{Join: true})
	var got map[string]any
	_ = json.Unmarshal([]byte(b.read(b.eng.Paths.ConfigFile())), &got)
	if got["ui_browser"] != `C:\Program Files\Microsoft\Edge\Application\msedge.exe` || got["selected_model_id"] != "glm-5.3" {
		t.Fatalf("config on B: %v", got)
	}
	if rep := b.run(Options{}); rep.Pushed != 0 {
		t.Fatalf("B pushed after applying: %+v", rep)
	}

	// Only the setting changed on A: nothing to send.
	a.write(a.eng.Paths.ConfigFile(), `{"api_base":"https://a.example","selected_model_id":"glm-5.3","ui_browser":"chromium"}`, future)
	if rep := a.run(Options{}); rep.Pushed != 0 {
		t.Fatalf("a change of the machine's own setting was pushed: %+v", rep)
	}

	// A machine without the setting stays without it, whatever arrives.
	c := newSide(t, stub, key)
	c.write(c.eng.Paths.ConfigFile(), `{"api_base":"https://c.example"}`, past)
	forged, _ := packFiles(map[string][]byte{"config.json": []byte(`{"api_base":"https://x","selected_model_id":"m2","ui_browser":"/tmp/not-a-browser"}`)})
	ct, _ := seal(key, KindConfig, NameConfig, forged)
	if _, err := a.eng.Client.Put(context.Background(), KindConfig, NameConfig, ct, stub.blob(KindConfig, NameConfig).version); err != nil {
		t.Fatal(err)
	}
	c.run(Options{Join: true})
	got = nil
	_ = json.Unmarshal([]byte(c.read(c.eng.Paths.ConfigFile())), &got)
	if _, ok := got["ui_browser"]; ok || got["selected_model_id"] != "m2" || got["api_base"] != "https://c.example" {
		t.Fatalf("config on C: %v", got)
	}
}
