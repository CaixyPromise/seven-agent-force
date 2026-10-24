package domain

import "testing"

func validFixture() (Manifest, []SourceFile) {
	return Manifest{
		SchemaVersion: PackageSchema, Name: "seven.demo.guard", TrustClass: "PLATFORM_TRUSTED",
		Runtime:     Runtime{Kind: "node", Requirement: ">=22 <23"},
		Entrypoints: []Entrypoint{{Event: "codex.PreToolUse", Path: "hooks/guard.mjs", TimeoutMS: 5000, FailureMode: "BLOCK"}},
		Limits:      Limits{StdinBytes: MaxInputBytes, StdoutBytes: MaxOutputBytes, StderrBytes: MaxErrorBytes},
	}, []SourceFile{{Path: "hooks/guard.mjs", Content: `process.stdout.write(JSON.stringify({schemaVersion:1,decision:"ALLOW",reasonCode:"OK",operatorMessage:"allowed",evidence:[]}))`}}
}

func TestCanonicalArchiveIsDeterministic(t *testing.T) {
	manifest, files := validFixture()
	first, firstHash, err := CanonicalArchive(manifest, files)
	if err != nil {
		t.Fatal(err)
	}
	second, secondHash, err := CanonicalArchive(manifest, files)
	if err != nil {
		t.Fatal(err)
	}
	if firstHash != secondHash || string(first) != string(second) {
		t.Fatal("canonical archive changed for identical source")
	}
}

func TestValidateRejectsTraversalAndDependencyInstall(t *testing.T) {
	manifest, files := validFixture()
	files = append(files, SourceFile{Path: "../escape.sh", Content: "echo escape"})
	result := Validate(manifest, files)
	if result.Valid || len(result.Errors) == 0 {
		t.Fatalf("unexpected validation: %+v", result)
	}
	files = append(files[:1], SourceFile{Path: "hooks/install.sh", Content: "npm install evil"})
	result = Validate(manifest, files)
	if result.Valid || len(result.Errors) == 0 {
		t.Fatalf("dependency installation was accepted: %+v", result)
	}
}

func TestValidateRejectsReservedManifestAndLifecycleScripts(t *testing.T) {
	manifest, files := validFixture()
	files = append(files, SourceFile{Path: "seven-hook-package.json", Content: `{}`})
	if result := Validate(manifest, files); result.Valid {
		t.Fatal("reserved canonical manifest path was accepted")
	}
	manifest, files = validFixture()
	files = append(files, SourceFile{Path: "package.json", Content: `{"scripts":{"postinstall":"node fetch.js"}}`})
	if result := Validate(manifest, files); result.Valid {
		t.Fatal("package lifecycle script was accepted")
	}
}

func TestValidateResultFailsClosed(t *testing.T) {
	if _, err := ValidateResult([]byte(`{"schemaVersion":1,"decision":"ASK","reasonCode":"NO"}`)); err == nil {
		t.Fatal("unsupported decision was accepted")
	}
	result, err := ValidateResult([]byte(`{"schemaVersion":1,"decision":"DENY","reasonCode":"SENTINEL_DENIED","operatorMessage":"blocked","evidence":[]}`))
	if err != nil || result.Decision != "DENY" {
		t.Fatalf("valid denial rejected: %+v %v", result, err)
	}
	if _, err := ValidateResult([]byte(`{"schemaVersion":1,"decision":"ALLOW","reasonCode":"OK","operatorMessage":"allowed","evidence":[]} {}`)); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
}
