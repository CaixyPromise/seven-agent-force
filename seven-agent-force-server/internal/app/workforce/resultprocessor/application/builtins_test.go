package application

import (
	"bytes"
	"strings"
	"testing"

	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

func TestBuiltinRuntimeSDKPublishesOnlySystemProcessors(t *testing.T) {
	assets := systemProcessorAssets()
	if len(assets) != 3 {
		t.Fatalf("system Processor catalog must stay small, got %d", len(assets))
	}
	wantNames := map[string]bool{OfficialDefaultProcessor: true, ReportProcessor: true, "开发方案处理器": true}
	seen := make(map[string]bool)
	for _, asset := range assets {
		t.Run(asset.Name, func(t *testing.T) {
			if !wantNames[asset.Name] || seen[asset.Name] {
				t.Fatal("system Processor name is unexpected or duplicated")
			}
			seen[asset.Name] = true
			if (asset.Name != "开发方案处理器" && asset.Version != builtinRuntimeSDKVersion) || asset.Manifest.Version != asset.Version {
				t.Fatal("system Processor release and manifest versions must match")
			}
			archive, digest, err := processor.CanonicalArchive(asset.Manifest, asset.Files)
			if err != nil {
				t.Fatal(err)
			}
			again, againDigest, err := processor.CanonicalArchive(asset.Manifest, asset.Files)
			if err != nil || digest != againDigest || !bytes.Equal(archive, again) {
				t.Fatal("system Processor bootstrap archive must be deterministic")
			}
		})
	}
}

func TestBuiltinSystemProcessorIdentitiesAreDirectAndStable(t *testing.T) {
	assets := systemProcessorAssets()
	if assets[0].Name != OfficialDefaultProcessor || assets[0].Manifest.Name != OfficialDefaultProcessor {
		t.Fatal("standard code Processor identity is not stable")
	}
	if assets[1].Name != ReportProcessor || assets[1].Manifest.Name != ReportProcessor {
		t.Fatal("report Processor identity is not stable")
	}
}

func TestBuiltinRuntimeSDKArchivesPreservePublishedSDKImports(t *testing.T) {
	for _, asset := range systemProcessorAssets() {
		t.Run(asset.Name, func(t *testing.T) {
			validation := processor.Validate(asset.Manifest, asset.Files)
			if !validation.Valid {
				t.Fatalf("invalid builtin: %v", validation.Errors)
			}
			archive, _, err := processor.CanonicalArchive(asset.Manifest, asset.Files)
			if err != nil {
				t.Fatal(err)
			}
			manifest, files, err := processor.ImportArchive(archive)
			if err != nil {
				t.Fatal(err)
			}
			if manifest.Version != asset.Version || len(files) != 1 || files[0].Path != "processor.py" {
				t.Fatal("published archive identity or entrypoint differs from builtin")
			}
			if (asset.Name != "开发方案处理器" && manifest.Version != "2.0.0") || !strings.Contains(files[0].Content, "from seven_workforce_runtime.processor import ") || strings.Contains(files[0].Content, "from seven_agent.force.processor import ") {
				t.Fatal("published 2.0.0 archive SDK imports changed during the product rename")
			}
		})
	}
}
