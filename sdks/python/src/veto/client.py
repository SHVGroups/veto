"""Synchronous and asynchronous clients for Veto's agent API."""

from __future__ import annotations

import asyncio
import os
import random
import time
import uuid
from collections.abc import Awaitable, Callable
from typing import Any, TypeVar

import httpx

from .errors import VetoError, VetoPending, VetoStopped
from .types import ActionResult, OutcomeStatus, Principal, Untrusted, VerifyResult

__all__ = ["Veto", "AsyncVeto"]

T = TypeVar("T")

#: The longest single long-poll the server allows; longer waits poll repeatedly.
MAX_POLL_SECONDS = 25
USER_AGENT = "shvgroups-veto-python/1.0.0"


def _settled(a: ActionResult) -> bool:
    return a["status"] not in ("pending_review", "evaluating")


def _wait_query(seconds: float | None) -> dict[str, int]:
    return {"wait": int(seconds)} if seconds and seconds >= 1 else {}


def _submit_body(
    tool: str,
    args: Any,
    request: str | None,
    principal: Principal | None,
    untrusted: list[Untrusted] | None,
    context: dict[str, Any] | None,
    idempotency_key: str | None,
) -> dict[str, Any]:
    body: dict[str, Any] = {"tool": tool, "args": args, "idempotency_key": idempotency_key or str(uuid.uuid4())}
    if request:
        body["request"] = request
    if principal:
        body["principal"] = principal
    if untrusted:
        body["untrusted"] = untrusted
    if context:
        body["context"] = context
    return body


def _error_from(res: httpx.Response) -> VetoError:
    try:
        data = res.json()
    except ValueError:
        data = {}
    err = (data or {}).get("error") or {}
    e = VetoError(
        res.status_code,
        err.get("code", "unknown"),
        err.get("message", f"Veto answered {res.status_code}"),
        err.get("details"),
        (data or {}).get("request_id"),
    )
    try:
        e.retry_after = float(res.headers.get("Retry-After", ""))
    except ValueError:
        pass
    return e


def _backoff(attempt: int, err: VetoError) -> float:
    if err.retry_after is not None:
        return err.retry_after
    return min(8.0, 0.25 * 2**attempt) * (0.5 + random.random() / 2)


class _Base:
    def __init__(self, api_key: str | None, base_url: str | None, timeout: float, max_retries: int):
        api_key = api_key or os.environ.get("VETO_API_KEY")
        base_url = base_url or os.environ.get("VETO_URL")
        if not api_key:
            raise ValueError("Veto: pass api_key or set VETO_API_KEY")
        if not base_url:
            raise ValueError("Veto: pass base_url or set VETO_URL")
        self._base_url = base_url.rstrip("/")
        self._headers = {"Authorization": f"Bearer {api_key}", "Accept": "application/json", "User-Agent": USER_AGENT}
        self._timeout = timeout
        self._max_retries = max_retries


class Veto(_Base):
    """A synchronous client.

    >>> veto = Veto()
    >>> result = veto.guard("gmail.send", args, lambda tool, args, grant: gmail.call(tool, args), request=user_message)
    """

    def __init__(
        self,
        api_key: str | None = None,
        base_url: str | None = None,
        *,
        timeout: float = 30.0,
        max_retries: int = 3,
        http_client: httpx.Client | None = None,
    ):
        super().__init__(api_key, base_url, timeout, max_retries)
        self._http = http_client or httpx.Client()

    def close(self) -> None:
        self._http.close()

    def __enter__(self) -> Veto:
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    def _request(self, method: str, path: str, *, json: Any = None, params: dict[str, Any] | None = None, wait: float = 0) -> Any:
        attempt = 0
        while True:
            try:
                res = self._http.request(
                    method, self._base_url + path, json=json, params=params, headers=self._headers, timeout=self._timeout + wait
                )
            except httpx.HTTPError as exc:
                err = VetoError(0, "network", f"Veto could not be reached: {exc}")
            else:
                if res.is_success:
                    return res.json() if res.content else None
                err = _error_from(res)
            if not err.retryable or attempt >= self._max_retries:
                raise err
            attempt += 1
            time.sleep(_backoff(attempt, err))

    def submit(
        self,
        tool: str,
        args: Any,
        *,
        request: str | None = None,
        principal: Principal | None = None,
        untrusted: list[Untrusted] | None = None,
        context: dict[str, Any] | None = None,
        idempotency_key: str | None = None,
        wait: float | None = None,
    ) -> ActionResult:
        """Proposes a call. With wait (seconds), the server holds the request until it is decided or the wait ends."""
        body = _submit_body(tool, args, request, principal, untrusted, context, idempotency_key)
        return self._request("POST", "/v1/actions", json=body, params=_wait_query(wait), wait=wait or 0)

    def get(self, action_id: str, *, wait: float | None = None) -> ActionResult:
        """Reads an action's current result, optionally waiting for a decision."""
        return self._request("GET", f"/v1/actions/{action_id}", params=_wait_query(wait), wait=wait or 0)

    def wait_for_decision(self, action_id: str, timeout: float = 60.0) -> ActionResult:
        """Waits until the action is decided or the timeout passes, and returns its latest state."""
        deadline = time.monotonic() + timeout
        while True:
            left = max(0.0, deadline - time.monotonic())
            action = self.get(action_id, wait=min(left, MAX_POLL_SECONDS))
            if _settled(action) or time.monotonic() >= deadline:
                return action

    def report_outcome(self, action_id: str, status: OutcomeStatus, detail: dict[str, Any] | None = None) -> dict[str, Any]:
        """Reports how a call went. Bad outcomes queue the decision for audit."""
        return self._request("POST", f"/v1/actions/{action_id}/outcome", json={"status": status, "detail": detail})

    def execute(self, action_id: str) -> dict[str, Any]:
        """Has Veto run an approved call through one of its HTTP connections, with the stored credential."""
        return self._request("POST", f"/v1/actions/{action_id}/execute", json={})

    def verify_grant(self, token: str, tool: str, args: Any, *, consume: bool = True) -> VerifyResult:
        """Checks a grant before running a call. Tool servers use this to refuse calls Veto did not approve."""
        return self._request("POST", "/v1/grants/verify", json={"token": token, "tool": tool, "args": args, "consume": consume})

    def guard(
        self,
        tool: str,
        args: Any,
        run: Callable[[str, Any, str | None], T],
        *,
        request: str | None = None,
        principal: Principal | None = None,
        untrusted: list[Untrusted] | None = None,
        context: dict[str, Any] | None = None,
        timeout: float = 60.0,
    ) -> T:
        """Asks Veto, waits for the decision, and runs the call only if it is approved.

        ``run`` receives the approved tool and arguments, which are the safer
        version when that was chosen, and the grant token. The outcome is
        reported afterwards. Raises VetoStopped when the call must not run and
        VetoPending when nobody decides within ``timeout`` seconds.
        """
        started = time.monotonic()
        action = self.submit(tool, args, request=request, principal=principal, untrusted=untrusted, context=context, wait=min(MAX_POLL_SECONDS, timeout))
        if not _settled(action):
            action = self.wait_for_decision(action["id"], timeout=max(0.0, timeout - (time.monotonic() - started)))
        if not _settled(action):
            raise VetoPending(action)
        decision = action.get("decision")
        if not decision or not decision.get("approved"):
            raise VetoStopped(action)
        grant = action.get("grant")
        try:
            result = run(decision.get("tool") or tool, decision.get("args", args), grant["token"] if grant else None)
        except Exception as exc:
            _quietly(lambda: self.report_outcome(action["id"], "failed", {"error": str(exc)}))
            raise
        _quietly(lambda: self.report_outcome(action["id"], "succeeded"))
        return result


class AsyncVeto(_Base):
    """An asynchronous client with the same methods as :class:`Veto`."""

    def __init__(
        self,
        api_key: str | None = None,
        base_url: str | None = None,
        *,
        timeout: float = 30.0,
        max_retries: int = 3,
        http_client: httpx.AsyncClient | None = None,
    ):
        super().__init__(api_key, base_url, timeout, max_retries)
        self._http = http_client or httpx.AsyncClient()

    async def aclose(self) -> None:
        await self._http.aclose()

    async def __aenter__(self) -> AsyncVeto:
        return self

    async def __aexit__(self, *exc: object) -> None:
        await self.aclose()

    async def _request(self, method: str, path: str, *, json: Any = None, params: dict[str, Any] | None = None, wait: float = 0) -> Any:
        attempt = 0
        while True:
            try:
                res = await self._http.request(
                    method, self._base_url + path, json=json, params=params, headers=self._headers, timeout=self._timeout + wait
                )
            except httpx.HTTPError as exc:
                err = VetoError(0, "network", f"Veto could not be reached: {exc}")
            else:
                if res.is_success:
                    return res.json() if res.content else None
                err = _error_from(res)
            if not err.retryable or attempt >= self._max_retries:
                raise err
            attempt += 1
            await asyncio.sleep(_backoff(attempt, err))

    async def submit(
        self,
        tool: str,
        args: Any,
        *,
        request: str | None = None,
        principal: Principal | None = None,
        untrusted: list[Untrusted] | None = None,
        context: dict[str, Any] | None = None,
        idempotency_key: str | None = None,
        wait: float | None = None,
    ) -> ActionResult:
        body = _submit_body(tool, args, request, principal, untrusted, context, idempotency_key)
        return await self._request("POST", "/v1/actions", json=body, params=_wait_query(wait), wait=wait or 0)

    async def get(self, action_id: str, *, wait: float | None = None) -> ActionResult:
        return await self._request("GET", f"/v1/actions/{action_id}", params=_wait_query(wait), wait=wait or 0)

    async def wait_for_decision(self, action_id: str, timeout: float = 60.0) -> ActionResult:
        deadline = time.monotonic() + timeout
        while True:
            left = max(0.0, deadline - time.monotonic())
            action = await self.get(action_id, wait=min(left, MAX_POLL_SECONDS))
            if _settled(action) or time.monotonic() >= deadline:
                return action

    async def report_outcome(self, action_id: str, status: OutcomeStatus, detail: dict[str, Any] | None = None) -> dict[str, Any]:
        return await self._request("POST", f"/v1/actions/{action_id}/outcome", json={"status": status, "detail": detail})

    async def execute(self, action_id: str) -> dict[str, Any]:
        return await self._request("POST", f"/v1/actions/{action_id}/execute", json={})

    async def verify_grant(self, token: str, tool: str, args: Any, *, consume: bool = True) -> VerifyResult:
        return await self._request("POST", "/v1/grants/verify", json={"token": token, "tool": tool, "args": args, "consume": consume})

    async def guard(
        self,
        tool: str,
        args: Any,
        run: Callable[[str, Any, str | None], Awaitable[T]],
        *,
        request: str | None = None,
        principal: Principal | None = None,
        untrusted: list[Untrusted] | None = None,
        context: dict[str, Any] | None = None,
        timeout: float = 60.0,
    ) -> T:
        """The asynchronous form of :meth:`Veto.guard`; ``run`` is awaited."""
        started = time.monotonic()
        action = await self.submit(tool, args, request=request, principal=principal, untrusted=untrusted, context=context, wait=min(MAX_POLL_SECONDS, timeout))
        if not _settled(action):
            action = await self.wait_for_decision(action["id"], timeout=max(0.0, timeout - (time.monotonic() - started)))
        if not _settled(action):
            raise VetoPending(action)
        decision = action.get("decision")
        if not decision or not decision.get("approved"):
            raise VetoStopped(action)
        grant = action.get("grant")
        try:
            result = await run(decision.get("tool") or tool, decision.get("args", args), grant["token"] if grant else None)
        except Exception as exc:
            try:
                await self.report_outcome(action["id"], "failed", {"error": str(exc)})
            except VetoError:
                pass
            raise
        try:
            await self.report_outcome(action["id"], "succeeded")
        except VetoError:
            pass
        return result


def _quietly(fn: Callable[[], object]) -> None:
    """Outcome reports are best effort: a failure to report must not hide the call's own result."""
    try:
        fn()
    except VetoError:
        pass
