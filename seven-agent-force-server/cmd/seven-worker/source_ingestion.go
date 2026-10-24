package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	gitdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	workspaceinputapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

const (
	maxSourceFiles = 100000
	maxSourceBytes = int64(2 << 30)
	maxSourceFile  = int64(256 << 20)
)

func (w *worker) runSourceIngestion(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage) error {
	claim, err := w.control.ClaimSourceIngestion(ctx, message)
	if err != nil {
		return err
	}
	fail := func(reason string) error {
		if reportErr := w.control.FailSourceIngestion(context.Background(), message, claim.LeaseToken, reason); reportErr != nil {
			return reportErr
		}
		return nil
	}
	root := filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "source-ingestion", strconv.FormatInt(claim.RunID, 10))
	if err = os.RemoveAll(root); err != nil {
		return fail("SOURCE_SPOOL_UNAVAILABLE")
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		return fail("SOURCE_SPOOL_UNAVAILABLE")
	}
	defer os.RemoveAll(root)
	repository := filepath.Join(root, "repository")
	if claim.Kind == gitdomain.SourceGitRepository {
		if err = w.ingestGitSource(ctx, message, claim, repository, root); err != nil {
			return fail(sourceReason(err))
		}
	} else {
		if err = w.ingestUploadedSource(ctx, claim, repository, root); err != nil {
			return fail(sourceReason(err))
		}
	}
	commit, err := controlledWorkerGit(ctx, repository, "rev-parse", "HEAD")
	if err != nil {
		return fail("SOURCE_COMMIT_INVALID")
	}
	tree, err := controlledWorkerGit(ctx, repository, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return fail("SOURCE_TREE_INVALID")
	}
	if err = rejectUnsupportedRepository(repository); err != nil {
		return fail(sourceReason(err))
	}
	bundle := filepath.Join(root, "repository.bundle")
	ref := "refs/seven/sources/" + strconv.FormatInt(claim.SourceRevisionID, 10)
	if _, err = controlledWorkerGit(ctx, repository, "update-ref", ref, commit); err != nil {
		return fail("SOURCE_BUNDLE_FAILED")
	}
	command := exec.CommandContext(ctx, "git", "-C", repository, "bundle", "create", bundle, ref)
	command.Env = controlledSourceGitEnv(root)
	if output, bundleErr := command.CombinedOutput(); bundleErr != nil || len(output) > 16<<10 {
		return fail("SOURCE_BUNDLE_FAILED")
	}
	if _, err = controlledWorkerGit(ctx, repository, "bundle", "verify", bundle); err != nil {
		return fail("SOURCE_BUNDLE_INVALID")
	}
	if err = verifySourceBundle(ctx, bundle, commit, tree, root); err != nil {
		return fail("SOURCE_BUNDLE_INVALID")
	}
	contentDigest, objectDigest, size, err := w.uploadSourceBundle(ctx, claim.BundleObjectKey, bundle)
	if err != nil {
		return fail(sourceReason(err))
	}
	return w.control.CompleteSourceIngestion(ctx, message, claim.LeaseToken, workspaceinputapp.SourceIngestionResult{ResolvedCommit: commit, ResolvedTree: tree, RepositoryBundleContentSHA256: contentDigest, RepositoryBundleObjectSHA256: objectDigest, RepositoryBundleSizeBytes: size})
}

func (w *worker) restoreTaskInput(ctx context.Context, item taskdomain.WorkItem, attempt taskdomain.Attempt) (attemptruntime.Workspace, string, error) {
	if item.TaskInput == nil || item.TaskInput.SourceCommit == "" || item.TaskInput.SourceTree == "" {
		return attemptruntime.Workspace{}, "", errors.New("TASK_INPUT_SNAPSHOT_MISSING")
	}
	workspace, err := w.runtime.PrepareWorkspace(item, attempt)
	if err != nil {
		return workspace, "", err
	}
	if err = os.MkdirAll(workspace.Repo, 0o700); err != nil {
		return workspace, "", err
	}
	if item.TaskInput.SourceKind == workspaceinputdomain.SourceKindEmptyWorkspace {
		if err = initializeEmptyTaskWorkspace(ctx, workspace.Repo, workspace.Temporary, *item.TaskInput); err != nil {
			return workspace, "", err
		}
		if err = os.Chmod(workspace.Repo, 0o777); err != nil {
			return workspace, "", err
		}
		if err = w.restoreTaskReferences(ctx, item, workspace); err != nil {
			return workspace, "", err
		}
		return workspace, item.TaskInput.SourceCommit, nil
	}
	if item.TaskInput.SourceKind != workspaceinputdomain.SourceKindSourceRevision || item.TaskInput.RepositoryBundleObjectKey == "" {
		return workspace, "", errors.New("TASK_INPUT_MODE_INVALID")
	}
	bundlePath := filepath.Join(workspace.Temporary, "source.bundle")
	if err = w.downloadVerifiedObject(ctx, item.TaskInput.RepositoryBundleObjectKey, item.TaskInput.RepositoryBundleObjectSHA256, item.TaskInput.RepositoryBundleContentSHA256, item.TaskInput.RepositoryBundleSizeBytes, bundlePath, maxSourceBytes); err != nil {
		return workspace, "", err
	}
	if _, err = controlledWorkerGit(ctx, workspace.Repo, "init", "-q"); err != nil {
		return workspace, "", err
	}
	ref := "refs/seven/task-input/" + strconv.FormatInt(item.TaskInput.SourceRevisionID, 10)
	if _, err = controlledWorkerGit(ctx, workspace.Repo, "-c", "protocol.file.allow=always", "fetch", "-q", bundlePath, "refs/*:refs/*"); err != nil {
		return workspace, "", errors.New("TASK_INPUT_BUNDLE_INVALID")
	}
	_ = ref
	if _, err = controlledWorkerGit(ctx, workspace.Repo, "checkout", "--detach", "-q", item.TaskInput.SourceCommit); err != nil {
		return workspace, "", errors.New("TASK_INPUT_COMMIT_MISSING")
	}
	tree, treeErr := controlledWorkerGit(ctx, workspace.Repo, "rev-parse", "HEAD^{tree}")
	if treeErr != nil || tree != item.TaskInput.SourceTree {
		return workspace, "", errors.New("TASK_INPUT_IDENTITY_MISMATCH")
	}
	if err = os.Chmod(workspace.Repo, 0o777); err != nil {
		return workspace, "", err
	}
	if err = w.restoreTaskReferences(ctx, item, workspace); err != nil {
		return workspace, "", err
	}
	return workspace, item.TaskInput.SourceCommit, nil
}

func initializeEmptyTaskWorkspace(ctx context.Context, repo, temp string, input taskdomain.TaskInputContract) error {
	expected := workspaceinputdomain.CanonicalEmptyWorkspaceIdentity()
	if input.SourceCommit != expected.Commit || input.SourceTree != expected.Tree || input.SourceIdentityDigest != expected.IdentityDigest || input.SourceRevisionID != 0 || input.RepositoryBundleSizeBytes != 0 {
		return errors.New("TASK_INPUT_IDENTITY_MISMATCH")
	}
	if _, err := controlledWorkerGit(ctx, repo, "init", "-q"); err != nil {
		return err
	}
	home, err := os.MkdirTemp(temp, ".empty-git-home-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	treeCommand := exec.CommandContext(ctx, "git", "-C", repo, "mktree")
	treeCommand.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+filepath.Join(home, "config"), "GIT_TERMINAL_PROMPT=0")
	treeOutput, treeErr := treeCommand.CombinedOutput()
	if treeErr != nil || strings.TrimSpace(string(treeOutput)) != expected.Tree {
		return errors.New("TASK_INPUT_EMPTY_TREE_INVALID")
	}
	command := exec.CommandContext(ctx, "git", "-C", repo, "hash-object", "-t", "commit", "-w", "--stdin")
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+filepath.Join(home, "config"), "GIT_TERMINAL_PROMPT=0")
	command.Stdin = strings.NewReader(string(workspaceinputdomain.EmptyWorkspaceCommitBody()))
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != expected.Commit {
		return errors.New("TASK_INPUT_EMPTY_COMMIT_INVALID")
	}
	if _, err = controlledWorkerGit(ctx, repo, "checkout", "--detach", "-q", expected.Commit); err != nil {
		return err
	}
	tree, err := controlledWorkerGit(ctx, repo, "rev-parse", "HEAD^{tree}")
	if err != nil || tree != expected.Tree {
		return errors.New("TASK_INPUT_IDENTITY_MISMATCH")
	}
	return nil
}

func (w *worker) restoreTaskReferences(ctx context.Context, item taskdomain.WorkItem, workspace attemptruntime.Workspace) error {
	if item.TaskInput == nil || len(item.TaskInput.References) == 0 {
		return nil
	}
	root := filepath.Join(workspace.InputRoot, "references")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	var index strings.Builder
	index.WriteString("# Task references\n\n")
	for _, ref := range item.TaskInput.References {
		safeName := filepath.Base(ref.DisplayName)
		if safeName == "." || safeName == "" {
			safeName = "attachment"
		}
		refID := strconv.FormatInt(ref.ID, 10)
		targetRoot := filepath.Join(root, refID)
		if err := os.MkdirAll(targetRoot, 0o755); err != nil {
			return err
		}
		target := filepath.Join(targetRoot, safeName)
		if err := w.downloadVerifiedObject(ctx, ref.ObjectKey, ref.ObjectSHA256, ref.ContentSHA256, ref.SizeBytes, target, maxSourceFile); err != nil {
			return err
		}
		if err := os.Chmod(target, 0o444); err != nil {
			return err
		}
		index.WriteString("- `" + refID + "/" + safeName + "` (" + ref.ContentType + ")\n")
	}
	return os.WriteFile(filepath.Join(workspace.InputRoot, "INPUTS.md"), []byte(index.String()), 0o444)
}

func (w *worker) downloadVerifiedObject(ctx context.Context, key, objectDigest, contentDigest string, size int64, target string, limit int64) error {
	if key == "" || size < 0 || size > limit || len(objectDigest) != 64 || len(contentDigest) != 64 {
		return errors.New("TASK_INPUT_OBJECT_IDENTITY_INVALID")
	}
	object, err := w.artifacts.Get(ctx, key)
	if err != nil {
		return errors.New("TASK_INPUT_OBJECT_UNAVAILABLE")
	}
	defer object.Close()
	if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	read, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(object, size+1))
	closeErr := file.Close()
	digest := hex.EncodeToString(hash.Sum(nil))
	if copyErr != nil || closeErr != nil || read != size || digest != objectDigest || digest != contentDigest {
		_ = os.Remove(target)
		return errors.New("TASK_INPUT_OBJECT_TAMPERED")
	}
	return nil
}

func (w *worker) ingestGitSource(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage, claim workspaceinputapp.SourceIngestionClaim, repository, root string) error {
	if err := os.MkdirAll(repository, 0o700); err != nil {
		return err
	}
	if _, err := controlledWorkerGit(ctx, repository, "init", "-q"); err != nil {
		return err
	}
	remote, env, allowFile, cleanup, err := w.prepareSourceRemote(ctx, message, claim, root)
	if err != nil {
		return err
	}
	defer cleanup()
	ref := sourceRemoteRef(claim.RequestedRefType, claim.RequestedRefName)
	if ref == "" {
		return errors.New("SOURCE_REF_INVALID")
	}
	commit := claim.RequestedRefName
	if claim.RequestedRefType != gitdomain.RefCommit {
		var readable bool
		commit, readable = readDeliveryRemoteRef(ctx, remote, ref, env, allowFile)
		if !readable || len(commit) != 40 {
			return errors.New("SOURCE_REF_NOT_FOUND")
		}
	} else if len(commit) != 40 {
		return errors.New("SOURCE_REF_INVALID")
	}
	if claim.RequestedRefType == gitdomain.RefTag {
		peeled, ok := readDeliveryRemoteRef(ctx, remote, ref+"^{}", env, allowFile)
		if ok && len(peeled) == 40 {
			commit = peeled
		}
	}
	if claim.FrozenCommit != "" && claim.FrozenCommit != commit {
		return errors.New("SOURCE_REF_IDENTITY_DRIFT")
	}
	if err = w.control.ObserveSourceCommit(ctx, message, claim.LeaseToken, commit); err != nil {
		return err
	}
	// The persisted bundle must be independently restorable. A depth-1 fetch
	// leaves a shallow boundary whose parent objects are absent; git bundle may
	// be created, but cannot be verified or restored without the original
	// remote once the selected commit has parents. Fetch the exact commit and
	// all of its reachable history instead.
	args := []string{"-C", repository, "-c", "protocol.file.allow=never", "fetch", "--no-tags", remote, commit}
	if allowFile {
		args[3] = "protocol.file.allow=always"
	}
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = env
	if output, fetchErr := command.CombinedOutput(); fetchErr != nil || len(output) > 32<<10 {
		return errors.New("SOURCE_REMOTE_UNREACHABLE")
	}
	if _, err = controlledWorkerGit(ctx, repository, "checkout", "--detach", "-q", commit); err != nil {
		return errors.New("SOURCE_COMMIT_INVALID")
	}
	return nil
}

func (w *worker) ingestUploadedSource(ctx context.Context, claim workspaceinputapp.SourceIngestionClaim, repository, root string) error {
	object, err := w.artifacts.Get(ctx, claim.InputObjectKey)
	if err != nil {
		return errors.New("SOURCE_OBJECT_UNAVAILABLE")
	}
	defer object.Close()
	archivePath := filepath.Join(root, "source.archive")
	file, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	read, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(object, claim.InputSizeBytes+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || read != claim.InputSizeBytes || hex.EncodeToString(hash.Sum(nil)) != claim.InputObjectSHA256 || claim.InputContentSHA256 != claim.InputObjectSHA256 {
		return errors.New("SOURCE_ARCHIVE_TAMPERED")
	}
	if err = extractUploadedProject(ctx, archivePath, repository); err != nil {
		return err
	}
	if err = filterUploadedProject(repository); err != nil {
		return err
	}
	if _, err = controlledWorkerGit(ctx, repository, "init", "-q"); err != nil {
		return err
	}
	if _, err = controlledWorkerGit(ctx, repository, "add", "-A", "--"); err != nil {
		return err
	}
	tree, err := controlledWorkerGit(ctx, repository, "write-tree")
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "git", "-C", repository, "commit-tree", tree)
	command.Stdin = strings.NewReader("Seven Agent Force uploaded source\n")
	// Use Git's portable RFC-style date syntax. The abbreviated `0 +0000`
	// form is rejected by newer Git builds, which would make every uploaded
	// source fail at synthetic commit creation even though the archive itself
	// passed validation.
	const syntheticCommitDate = "Thu, 01 Jan 1970 00:00:00 +0000"
	command.Env = append(controlledSourceGitEnv(root), "GIT_AUTHOR_NAME=Seven Agent Force", "GIT_AUTHOR_EMAIL=seven-workforce@localhost", "GIT_AUTHOR_DATE="+syntheticCommitDate, "GIT_COMMITTER_NAME=Seven Agent Force", "GIT_COMMITTER_EMAIL=seven-workforce@localhost", "GIT_COMMITTER_DATE="+syntheticCommitDate)
	output, err := command.CombinedOutput()
	if err != nil {
		return errors.New("SOURCE_SYNTHETIC_COMMIT_FAILED")
	}
	commit := strings.TrimSpace(string(output))
	if _, err = controlledWorkerGit(ctx, repository, "update-ref", "HEAD", commit); err != nil {
		return err
	}
	if _, err = controlledWorkerGit(ctx, repository, "checkout", "--detach", "-q", commit); err != nil {
		return err
	}
	return nil
}

func (w *worker) prepareSourceRemote(ctx context.Context, message workspaceinputdomain.SourceIngestionMessage, claim workspaceinputapp.SourceIngestionClaim, root string) (string, []string, bool, func(), error) {
	clean := func() {}
	connection := claim.Repository
	if connection.Transport == gitdomain.TransportLocalBare {
		remote, err := validatedLocalBarePath(w.cfg.Worker.WorkspaceRoot, connection.RepositoryPath)
		if err != nil {
			return "", nil, false, clean, errors.New("SOURCE_LOCAL_REPOSITORY_OUTSIDE_CONTROLLED_ROOT")
		}
		return remote, controlledSourceGitEnv(root), true, clean, err
	}
	addresses, err := netResolve(ctx, connection.Host)
	if err != nil || connection.Egress.Authorize(connection.Host, connection.Port, addresses) != nil {
		return "", nil, false, clean, errors.New("SOURCE_EGRESS_DENIED")
	}
	credential, err := w.control.MaterializeSourceCredential(ctx, message, claim.LeaseToken)
	if err != nil {
		return "", nil, false, clean, errors.New("SOURCE_CREDENTIAL_MATERIALIZATION_FAILED")
	}
	return prepareSourceAuthenticationPinned(ctx, root, connection, credential, addresses)
}

func prepareSourceAuthentication(ctx context.Context, root string, connection gitdomain.RepositoryTarget, credential workspaceinputapp.SourceCredential) (string, []string, bool, func(), error) {
	return prepareSourceAuthenticationPinned(ctx, root, connection, credential, nil)
}

func prepareSourceAuthenticationPinned(ctx context.Context, root string, connection gitdomain.RepositoryTarget, credential workspaceinputapp.SourceCredential, resolvedIPs []string) (string, []string, bool, func(), error) {
	clean := func() {}
	credentialRoot := filepath.Join(root, "credentials")
	if err := os.MkdirAll(credentialRoot, 0o700); err != nil {
		return "", nil, false, clean, err
	}
	clean = func() { _ = os.RemoveAll(credentialRoot) }
	env := controlledSourceGitEnv(root)
	switch connection.Transport {
	case gitdomain.TransportHTTP, gitdomain.TransportHTTPS:
		if len(resolvedIPs) > 0 {
			env = append(env, "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.curloptResolve", "GIT_CONFIG_VALUE_0="+sourceCurlResolve(connection.Host, connection.Port, resolvedIPs[0]))
		}
		if credential.Username == "" && credential.Password == "" {
			return sourceHTTPRemote(connection), env, false, clean, nil
		}
		username, password, askpass := filepath.Join(credentialRoot, "username"), filepath.Join(credentialRoot, "password"), filepath.Join(credentialRoot, "askpass")
		if credential.Username == "" || credential.Password == "" {
			return "", nil, false, clean, errors.New("SOURCE_CREDENTIAL_INVALID")
		}
		if os.WriteFile(username, []byte(credential.Username), 0o600) != nil || os.WriteFile(password, []byte(credential.Password), 0o600) != nil || os.WriteFile(askpass, []byte("#!/bin/sh\ncase \"$1\" in *sername*) exec /bin/cat \"$SEVEN_GIT_USERNAME_FILE\";; *) exec /bin/cat \"$SEVEN_GIT_PASSWORD_FILE\";; esac\n"), 0o700) != nil {
			return "", nil, false, clean, errors.New("SOURCE_CREDENTIAL_MATERIALIZATION_FAILED")
		}
		env = append(env, "GIT_ASKPASS="+askpass, "SEVEN_GIT_USERNAME_FILE="+username, "SEVEN_GIT_PASSWORD_FILE="+password)
		return sourceHTTPRemote(connection), env, false, clean, nil
	case gitdomain.TransportSSH:
		key, known := filepath.Join(credentialRoot, "identity"), filepath.Join(credentialRoot, "known_hosts")
		if credential.PrivateKey == "" || credential.KnownHosts == "" || connection.SSHKnownHostFingerprint == "" {
			return "", nil, false, clean, errors.New("SOURCE_CREDENTIAL_INVALID")
		}
		if os.WriteFile(key, []byte(credential.PrivateKey), 0o600) != nil || os.WriteFile(known, []byte(credential.KnownHosts), 0o600) != nil {
			return "", nil, false, clean, errors.New("SOURCE_CREDENTIAL_MATERIALIZATION_FAILED")
		}
		fingerprint, fingerprintErr := exec.CommandContext(ctx, "ssh-keygen", "-lf", known).Output()
		if fingerprintErr != nil || !strings.Contains(string(fingerprint), strings.TrimSpace(connection.SSHKnownHostFingerprint)) {
			return "", nil, false, clean, errors.New("SOURCE_SSH_HOST_IDENTITY_MISMATCH")
		}
		command, commandErr := sourceSSHCommand(credentialRoot, key, known, connection.Host, connection.Port, resolvedIPs)
		if commandErr != nil {
			return "", nil, false, clean, commandErr
		}
		bridgeEnv, bridgeErr := appendDockerBridgeEnvironment(ctx, env)
		if bridgeErr != nil {
			return "", nil, false, clean, bridgeErr
		}
		env = append(bridgeEnv, "GIT_SSH_COMMAND="+command)
		return sourceSSHRemote(connection), env, false, clean, nil
	default:
		return "", nil, false, clean, errors.New("SOURCE_TRANSPORT_UNSUPPORTED")
	}
}

func sourceCurlResolve(host string, port int, address string) string {
	if strings.Contains(address, ":") {
		address = "[" + address + "]"
	}
	return fmt.Sprintf("%s:%d:%s", host, port, address)
}

// sourceSSHCommand pins SSH's TCP connection to the exact addresses validated
// by the Repository Connection egress policy. SSH still verifies the original
// host name through HostKeyAlias, so pinning cannot weaken host-key checks.
func sourceSSHCommand(root, key, known, host string, port int, resolvedIPs []string) (string, error) {
	if len(resolvedIPs) == 0 {
		return "ssh -F /dev/null -i " + key + " -o UserKnownHostsFile=" + known + " -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes", nil
	}
	ip := net.ParseIP(strings.TrimSpace(resolvedIPs[0]))
	if ip == nil || port < 1 || port > 65535 || strings.TrimSpace(host) == "" {
		return "", errors.New("SOURCE_EGRESS_PIN_INVALID")
	}
	alias := host
	if port != 22 {
		alias = "[" + host + "]:" + strconv.Itoa(port)
	}
	proxy, proxyErr := sshEgressProxyCommand(root, ip, port)
	if proxyErr != nil {
		return "", errors.New("SOURCE_EGRESS_PIN_UNAVAILABLE")
	}
	script := filepath.Join(root, "pinned-ssh")
	body := "#!/bin/sh\nexec ssh -F /dev/null -i " + shellQuote(key) +
		" -o " + shellQuote("UserKnownHostsFile="+known) +
		" -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes" +
		" -o " + shellQuote("HostKeyAlias="+alias) +
		" -o " + shellQuote("ProxyCommand="+proxy) + " \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return "", errors.New("SOURCE_EGRESS_PIN_UNAVAILABLE")
	}
	return script, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}

func (w *worker) uploadSourceBundle(ctx context.Context, key, path string) (string, string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() < 1 || info.Size() > maxSourceBytes {
		return "", "", 0, errors.New("SOURCE_BUNDLE_SIZE_INVALID")
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", "", 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", "", 0, err
	}
	if err = w.artifacts.Put(ctx, key, "application/x-git-bundle", file, info.Size()); err != nil {
		return "", "", 0, errors.New("SOURCE_UPLOAD_FAILED")
	}
	readback, err := w.artifacts.Get(ctx, key)
	if err != nil {
		return "", "", 0, errors.New("SOURCE_READBACK_FAILED")
	}
	defer readback.Close()
	readHash := sha256.New()
	size, err := io.Copy(readHash, io.LimitReader(readback, info.Size()+1))
	if err != nil || size != info.Size() || hex.EncodeToString(readHash.Sum(nil)) != digest {
		return "", "", 0, errors.New("SOURCE_READBACK_FAILED")
	}
	return digest, digest, info.Size(), nil
}

func verifySourceBundle(ctx context.Context, bundle, commit, tree, root string) error {
	verify := filepath.Join(root, "verify")
	if err := os.MkdirAll(verify, 0o700); err != nil {
		return err
	}
	if _, err := controlledWorkerGit(ctx, verify, "init", "-q"); err != nil {
		return err
	}
	if _, err := controlledWorkerGit(ctx, verify, "-c", "protocol.file.allow=always", "fetch", "-q", bundle, "refs/*:refs/*"); err != nil {
		return err
	}
	if _, err := controlledWorkerGit(ctx, verify, "checkout", "--detach", "-q", commit); err != nil {
		return err
	}
	actual, err := controlledWorkerGit(ctx, verify, "rev-parse", "HEAD^{tree}")
	return identityEqual(actual, tree, err)
}

func identityEqual(actual, expected string, err error) error {
	if err != nil || actual != expected {
		return errors.New("SOURCE_IDENTITY_MISMATCH")
	}
	return nil
}
func sourceRemoteRef(kind, name string) string {
	switch kind {
	case gitdomain.RefBranch:
		return "refs/heads/" + name
	case gitdomain.RefTag:
		return "refs/tags/" + name
	case gitdomain.RefCommit:
		return name
	default:
		return ""
	}
}
func controlledSourceGitEnv(root string) []string {
	return appendRuntimeGitCA([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "XDG_CONFIG_HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_ATTR_NOSYSTEM=1", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C"})
}
func sourceReason(err error) string {
	if err == nil {
		return ""
	}
	value := strings.SplitN(err.Error(), ":", 2)[0]
	if strings.HasPrefix(value, "SOURCE_") && len(value) <= 128 {
		return value
	}
	return "SOURCE_INGESTION_FAILED"
}

func rejectUnsupportedRepository(root string) error {
	if _, err := os.Stat(filepath.Join(root, ".gitmodules")); err == nil {
		return errors.New("SOURCE_SUBMODULE_UNSUPPORTED")
	}
	command := exec.Command("git", "-C", root, "ls-files", "-s")
	output, err := command.Output()
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "160000 ") {
			return errors.New("SOURCE_SUBMODULE_UNSUPPORTED")
		}
	}
	files := exec.Command("git", "-C", root, "ls-files", "-z")
	body, err := files.Output()
	if err != nil {
		return err
	}
	for _, raw := range strings.Split(string(body), "\x00") {
		if raw == "" {
			continue
		}
		file, openErr := os.Open(filepath.Join(root, filepath.FromSlash(raw)))
		if openErr != nil {
			continue
		}
		reader := bufio.NewReader(io.LimitReader(file, 256))
		first, _ := reader.ReadString('\n')
		_ = file.Close()
		if strings.TrimSpace(first) == "version https://git-lfs.github.com/spec/v1" {
			return errors.New("SOURCE_GIT_LFS_UNSUPPORTED")
		}
	}
	return nil
}

func extractUploadedProject(ctx context.Context, archivePath, destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	header := make([]byte, 4)
	_, _ = io.ReadFull(file, header)
	_, _ = file.Seek(0, io.SeekStart)
	if string(header[:2]) == "PK" {
		return extractSourceZIP(ctx, file, destination)
	}
	gz, err := gzip.NewReader(file)
	if err != nil {
		return errors.New("SOURCE_ARCHIVE_FORMAT_UNSUPPORTED")
	}
	defer gz.Close()
	return extractSourceTar(ctx, tar.NewReader(gz), destination)
}

func extractSourceZIP(ctx context.Context, source *os.File, destination string) error {
	info, _ := source.Stat()
	reader, err := zip.NewReader(source, info.Size())
	if err != nil {
		return errors.New("SOURCE_ARCHIVE_INVALID")
	}
	total := int64(0)
	seen := map[string]struct{}{}
	for index, item := range reader.File {
		if index >= maxSourceFiles {
			return errors.New("SOURCE_ARCHIVE_TOO_LARGE")
		}
		path, err := safeSourcePath(item.Name, seen)
		if err != nil {
			return err
		}
		if item.Mode()&os.ModeSymlink != 0 || !item.FileInfo().Mode().IsRegular() && !item.FileInfo().IsDir() || item.UncompressedSize64 > uint64(maxSourceFile) || item.Mode()&06000 != 0 {
			return errors.New("SOURCE_ARCHIVE_UNSAFE_ENTRY")
		}
		if item.CompressedSize64 > 0 && item.UncompressedSize64/item.CompressedSize64 > 1000 {
			return errors.New("SOURCE_ARCHIVE_COMPRESSION_RATIO_EXCEEDED")
		}
		if item.FileInfo().IsDir() {
			if err = os.MkdirAll(filepath.Join(destination, path), 0o700); err != nil {
				return err
			}
			continue
		}
		total += int64(item.UncompressedSize64)
		if total > maxSourceBytes {
			return errors.New("SOURCE_ARCHIVE_TOO_LARGE")
		}
		input, err := item.Open()
		if err != nil {
			return err
		}
		err = writeSourceFile(ctx, input, filepath.Join(destination, path), int64(item.UncompressedSize64))
		_ = input.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractSourceTar(ctx context.Context, reader *tar.Reader, destination string) error {
	total, count, seen := int64(0), 0, map[string]struct{}{}
	for {
		item, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("SOURCE_ARCHIVE_INVALID")
		}
		count++
		if count > maxSourceFiles {
			return errors.New("SOURCE_ARCHIVE_TOO_LARGE")
		}
		path, err := safeSourcePath(item.Name, seen)
		if err != nil {
			return err
		}
		if item.Typeflag == tar.TypeDir {
			if err = os.MkdirAll(filepath.Join(destination, path), 0o700); err != nil {
				return err
			}
			continue
		}
		if item.Typeflag != tar.TypeReg || item.Size < 0 || item.Size > maxSourceFile || item.Mode&06000 != 0 {
			return errors.New("SOURCE_ARCHIVE_UNSAFE_ENTRY")
		}
		total += item.Size
		if total > maxSourceBytes {
			return errors.New("SOURCE_ARCHIVE_TOO_LARGE")
		}
		if err = writeSourceFile(ctx, reader, filepath.Join(destination, path), item.Size); err != nil {
			return err
		}
	}
	return nil
}

func safeSourcePath(raw string, seen map[string]struct{}) (string, error) {
	if !utf8.ValidString(raw) {
		return "", errors.New("SOURCE_ARCHIVE_INVALID_PATH")
	}
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
	folded := strings.ToLower(norm.NFC.String(clean))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains("/"+clean+"/", "/.git/") || strings.ContainsRune(clean, 0) {
		return "", errors.New("SOURCE_ARCHIVE_INVALID_PATH")
	}
	if _, ok := seen[folded]; ok {
		return "", errors.New("SOURCE_ARCHIVE_PATH_COLLISION")
	}
	seen[folded] = struct{}{}
	return filepath.FromSlash(clean), nil
}
func writeSourceFile(ctx context.Context, reader io.Reader, path string, expected int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(reader, expected+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || written != expected {
		return errors.New("SOURCE_ARCHIVE_TRUNCATED")
	}
	return nil
}

func filterUploadedProject(root string) error {
	regenerable := map[string]struct{}{"node_modules": {}, "target": {}, "__pycache__": {}, ".pytest_cache": {}, ".mypy_cache": {}, ".ruff_cache": {}, ".venv": {}, "venv": {}, "dist": {}, "build": {}, ".next": {}, ".nuxt": {}, "coverage": {}, ".gradle": {}, "bin": {}, "obj": {}, "tmp": {}, "temp": {}}
	var remove []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		relative, _ := filepath.Rel(root, path)
		base := entry.Name()
		if entry.IsDir() {
			if _, ok := regenerable[base]; ok {
				remove = append(remove, path)
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(base, ".env") || strings.EqualFold(base, "id_rsa") || strings.HasSuffix(strings.ToLower(base), ".pem") {
			return errors.New("SOURCE_SECRET_FILE_FORBIDDEN")
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Mode()&06000 != 0 || info.Size() > maxSourceFile {
			return errors.New("SOURCE_ARCHIVE_UNSAFE_ENTRY")
		}
		_ = relative
		return nil
	})
	if err != nil {
		return err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(remove)))
	for _, path := range remove {
		if err = os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
}

func sourceHTTPRemote(connection gitdomain.RepositoryTarget) string {
	host := connection.Host
	defaultPort := 443
	scheme := "https"
	if connection.Transport == gitdomain.TransportHTTP {
		defaultPort = 80
		scheme = "http"
	}
	if connection.Port != defaultPort {
		host += ":" + strconv.Itoa(connection.Port)
	}
	return scheme + "://" + host + "/" + strings.TrimPrefix(connection.RepositoryPath, "/")
}
func sourceSSHRemote(connection gitdomain.RepositoryTarget) string {
	host := connection.Host
	if connection.Port != 22 {
		host += ":" + strconv.Itoa(connection.Port)
	}
	return "ssh://git@" + host + "/" + strings.TrimPrefix(connection.RepositoryPath, "/")
}
func netResolve(ctx context.Context, host string) ([]string, error) {
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	out := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))
	for _, item := range addresses {
		value := item.String()
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return orderResolvedIPs(out), err
}

// orderResolvedIPs produces a stable pin set for a connection.  Docker Desktop
// commonly resolves *.localhost to both ::1 and 127.0.0.1 while its published
// Git SSH port is intentionally bound only on IPv4.  We therefore prefer IPv4
// when both address families were authorised, rather than treating resolver
// order as a transport decision.  The selected address still has to pass the
// frozen connection egress policy before it reaches SSH.
func orderResolvedIPs(addresses []string) []string {
	ordered := append([]string(nil), addresses...)
	sort.SliceStable(ordered, func(left, right int) bool {
		leftIPv4 := net.ParseIP(ordered[left]).To4() != nil
		rightIPv4 := net.ParseIP(ordered[right]).To4() != nil
		if leftIPv4 != rightIPv4 {
			return leftIPv4
		}
		return ordered[left] < ordered[right]
	})
	return ordered
}
