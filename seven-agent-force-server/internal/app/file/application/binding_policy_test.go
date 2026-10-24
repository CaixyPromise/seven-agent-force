package application

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
)

func TestValidateBindableFileRejectsUnsafeAvatarCandidates(t *testing.T) {
	base := &domain.FileInfo{
		ID:              1,
		ContentType:     "image/png",
		Status:          domain.FileStatusAvailable,
		ScanStatus:      domain.ScanStatusClean,
		IntegrityStatus: domain.IntegrityVerified,
	}
	cases := []struct {
		name string
		edit func(*domain.FileInfo)
	}{
		{name: "deleted", edit: func(file *domain.FileInfo) { file.IsDeleted = 1 }},
		{name: "quarantined", edit: func(file *domain.FileInfo) { file.Status = domain.FileStatusQuarantined }},
		{name: "blank scan", edit: func(file *domain.FileInfo) { file.ScanStatus = "" }},
		{name: "infected", edit: func(file *domain.FileInfo) { file.ScanStatus = domain.ScanStatusInfected }},
		{name: "blank integrity", edit: func(file *domain.FileInfo) { file.IntegrityStatus = "" }},
		{name: "integrity mismatch", edit: func(file *domain.FileInfo) { file.IntegrityStatus = domain.IntegrityMismatch }},
		{name: "non image", edit: func(file *domain.FileInfo) { file.ContentType = "text/plain" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := *base
			tc.edit(&file)
			if err := validateBindableFile(&file, filefacade.UserAvatar); err == nil {
				t.Fatalf("expected unsafe avatar candidate to be rejected")
			}
		})
	}
}

func TestValidateBindableFileAllowsCleanVerifiedAvatar(t *testing.T) {
	file := &domain.FileInfo{
		ID:              1,
		ContentType:     "image/webp",
		Status:          domain.FileStatusAvailable,
		ScanStatus:      domain.ScanStatusClean,
		IntegrityStatus: domain.IntegrityVerified,
	}
	if err := validateBindableFile(file, filefacade.UserAvatar); err != nil {
		t.Fatalf("expected clean verified avatar file to bind: %v", err)
	}
}
