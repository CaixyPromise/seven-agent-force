# License evidence

`licenses.json` is an exact-version inventory; `licenses/` retains source license,
notice and attribution text by SHA256. `THIRD_PARTY_NOTICES.md` at repository root
states the scope decisions, FTL selection and MPL source availability.

Checks: `python3 scripts/release/check_licenses.py`. The release script checks
the hashes of dependency inputs and all locked dependencies, rejects production
promotions, new unreviewed versions/licenses or changed
notice hashes, annotates the SBOM, copies these notices and downloads the exact
checksum-pinned corresponding source for runtime MPL modules.

Refresh after a dependency change: create a Syft JSON inventory with Go license
enrichment, export the active Web production tree with `pnpm list --prod --depth
Infinity --json`, then use `scripts/release/collect_licenses.py <sbom> <tree>`.
Inspect its diff and every unresolved entry; generation is evidence collection,
not automatic permission to add a new license. Build-only declarations cannot
become runtime notices without full attribution evidence. Review source-license
changes and regenerate the dependency table in THIRD_PARTY_NOTICES.md.

Only source archives needed for binary MPL compliance are downloaded at release
time; no third-party program is executed by these tools. No Docker image or
Coding CLI distribution is covered by this inventory.
