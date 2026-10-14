"""Errors raised by the Veto client."""

from __future__ import annotations

from typing import Any

from .types import ActionResult


class VetoError(Exception):
    """An error response from the Veto API, or a failure to reach it (status 0)."""

    def __init__(self, status: int, code: str, message: str, details: Any = None, request_id: str | None = None):
        super().__init__(message)
        self.status = status
        self.code = code
        self.message = message
        self.details = details
        self.request_id = request_id
        self.retry_after: float | None = None

    @property
    def retryable(self) -> bool:
        """Whether trying the same request again may succeed."""
        return self.status == 0 or self.status == 429 or self.status >= 500


class VetoStopped(Exception):
    """Veto decided the action must not run."""

    def __init__(self, action: ActionResult):
        reasons = (action.get("decision") or {}).get("reasons") or action.get("hold_reasons") or []
        text = "; ".join(f"{r['code']} ({r['detail']})" if r.get("detail") else r["code"] for r in reasons)
        super().__init__(f"Veto stopped {action['tool']}" + (f": {text}" if text else ""))
        self.action = action


class VetoPending(Exception):
    """The action is still waiting for a person after the time the caller allowed."""

    def __init__(self, action: ActionResult):
        where = f" at {action['review_url']}" if action.get("review_url") else ""
        super().__init__(f"{action['tool']} is waiting for review{where}; check again later with get({action['id']!r})")
        self.action = action
