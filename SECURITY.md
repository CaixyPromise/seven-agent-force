# Security Policy

## Reporting a vulnerability

Report suspected vulnerabilities privately through [GitHub private vulnerability reporting](https://github.com/CaixyPromise/seven-agent-force/security/advisories/new). Do not open a public issue containing an exposed credential or exploit details.

Include the affected version, reproducible steps, impact, and any proposed mitigation. Do not include production credentials, personal data, or data copied from systems you do not own.

## Disclosure process

Maintainers will acknowledge a valid report, assess affected versions, coordinate a fix, and publish an advisory after supported releases are available. Public disclosure should be coordinated so operators have a reasonable upgrade window.

## Credential handling

Never commit real database DSNs, API tokens, private keys, third-party application credentials, environment files, database exports, uploads, or production topology. Example configuration must contain empty values or documented placeholders only. Rotate any credential that has entered Git history; deleting it from the latest commit is not sufficient.

## Dependency scan policy

CI and release packaging run `govulncheck` and fail for every reachable finding except two explicitly reviewed upstream Moby findings: `GO-2026-4883` and `GO-2026-4887`. Those advisories concern Moby daemon plugin privilege and authorization validation. Seven Agent Force imports the Docker API client and does not embed or run the affected daemon-side validation paths. The current stable `github.com/docker/docker` module has no fixed release for either advisory, so this exception is intentionally narrow, visible in `scripts/check-go-vulnerabilities.sh`, and must be re-evaluated whenever the Docker dependency changes. Any additional reachable finding fails the build.
