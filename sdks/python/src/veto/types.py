"""Shapes of the JSON the Veto API returns. Field names match the API."""

from __future__ import annotations

from typing import Any, Literal, TypedDict

Option = Literal["proceed", "proceed_safer", "stop"]
ActionStatus = Literal["evaluating", "pending_review", "approved", "denied", "expired"]
DecidedBy = Literal["model", "human", "policy", "failsafe", "expiry"]
OutcomeStatus = Literal["succeeded", "failed", "reversed", "complaint"]


class Reason(TypedDict, total=False):
    code: str
    detail: str


class Principal(TypedDict, total=False):
    id: str
    display: str


class Untrusted(TypedDict):
    source: str
    content: str


class Decision(TypedDict, total=False):
    option: Option
    approved: bool
    tool: str
    args: Any
    decided_by: DecidedBy
    reasons: list[Reason]
    confidence: float | None
    note: str


class Grant(TypedDict):
    token: str
    expires_at: str


class OptionSpec(TypedDict, total=False):
    key: Option
    available: bool
    tool: str
    args: Any
    changes: list[str]


class ActionResult(TypedDict, total=False):
    id: str
    status: ActionStatus
    tool: str
    decision: Decision | None
    grant: Grant | None
    hold_reasons: list[Reason]
    options: list[OptionSpec]
    review_url: str
    review_deadline: str | None
    received_at: str
    decided_at: str | None


class VerifyResult(TypedDict):
    valid: bool
    grant_id: str
    action_id: str
    option: Option
    expires_at: str
    consumed: bool
