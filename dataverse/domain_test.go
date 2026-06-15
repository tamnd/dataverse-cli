package dataverse

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in dataverse_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "dataverse" {
		t.Errorf("Scheme = %q, want dataverse", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "dataverse" {
		t.Errorf("Identity.Binary = %q, want dataverse", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	typ, id, err := Domain{}.Classify("doi:10.7910/DVN/FBX7BF")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if typ != "dataset" {
		t.Errorf("type = %q, want dataset", typ)
	}
	if id != "doi:10.7910/DVN/FBX7BF" {
		t.Errorf("id = %q, want doi:10.7910/DVN/FBX7BF", id)
	}
}

func TestClassifyDOIURL(t *testing.T) {
	typ, id, err := Domain{}.Classify("https://doi.org/10.7910/DVN/FBX7BF")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if typ != "dataset" {
		t.Errorf("type = %q, want dataset", typ)
	}
	if id != "doi:10.7910/DVN/FBX7BF" {
		t.Errorf("id = %q, want doi:10.7910/DVN/FBX7BF", id)
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("dataset", "doi:10.7910/DVN/FBX7BF")
	want := "https://doi.org/10.7910/DVN/FBX7BF"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateNonDOI(t *testing.T) {
	got, err := Domain{}.Locate("dataset", "hdl:1234/567")
	want := "https://dataverse.harvard.edu/dataset.xhtml?persistentId=hdl:1234/567"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknown(t *testing.T) {
	_, err := Domain{}.Locate("file", "abc")
	if err == nil {
		t.Error("expected error for unknown resource type")
	}
}

