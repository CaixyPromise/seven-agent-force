"""Image-owned Processor helpers, isolated from AgentAdapter execution.

The helpers create ordinary Result Contract dictionaries.  They intentionally
do not perform network, object-store, credential, Task-state or delivery work.
Those capabilities remain owned by the Supervisor and Server coordinators.
"""

from __future__ import annotations

import os
from typing import Any


def _result(decision: str, *, reason_code: str = "OK", message: str = "", **fields: Any) -> dict[str, Any]:
    result: dict[str, Any] = {"decision": decision, "reasonCode": reason_code, "message": message}
    for key in ("checks", "findings", "questions", "outputs", "actions"):
        if key in fields and fields[key] is not None:
            result[key] = fields[key]
    return result


def pass_result(*, reason_code: str = "OK", message: str = "", checks=None, findings=None, outputs=None, actions=None) -> dict[str, Any]:
    return _result("PASS", reason_code=reason_code, message=message, checks=checks, findings=findings, outputs=outputs, actions=actions)


def ask_agent(*, reason_code: str, message: str, checks=None, findings=None) -> dict[str, Any]:
    return _result("REVISE", reason_code=reason_code, message=message, checks=checks, findings=findings)


def wait_user(*, reason_code: str, message: str, questions=None, findings=None, outputs=None) -> dict[str, Any]:
    return _result("WAIT_USER", reason_code=reason_code, message=message, questions=questions, findings=findings, outputs=outputs)


def reject(*, reason_code: str, message: str, checks=None, findings=None) -> dict[str, Any]:
    return _result("REJECT", reason_code=reason_code, message=message, checks=checks, findings=findings)


def manual_review(*, reason_code: str, message: str, checks=None, findings=None, questions=None, outputs=None) -> dict[str, Any]:
    return _result("MANUAL_REVIEW", reason_code=reason_code, message=message, checks=checks, findings=findings, questions=questions, outputs=outputs)


def workspace_path(*, name: str, path: str, storage: str = "IMMEDIATE") -> dict[str, str]:
    return {"name": name, "kind": "PATH", "path": path, "storage": storage}


def archive_output(
    *,
    name: str,
    files: list[str],
    excludes: list[str] | None = None,
    format: str = "ZIP",
    storage: str = "ON_DEMAND",
) -> dict[str, Any]:
    """Declare a platform-built archive from the frozen trusted result.

    ``files`` and ``excludes`` are selection patterns, not a self-reported
    manifest. The Result Coordinator resolves the concrete files and the
    post-process Worker creates deterministic ZIP/TAR.GZ bytes later.
    """

    return {
        "name": name,
        "kind": "ARCHIVE",
        "selection": {"includes": list(files), "excludes": list(excludes or [])},
        "archiveFormat": format,
        "storage": storage,
    }


def get_env(name: str, default_value: str | None = None) -> str | None:
    """Return a cycle-scoped CONFIG/SECRET value approved by the platform.

    Values are present only while ``process(context)`` runs.  The helper is a
    convenience for authoring templates; it does not widen the Processor's
    authority beyond the environment bindings frozen in the Task snapshot.
    """

    return os.getenv(name, default_value)
