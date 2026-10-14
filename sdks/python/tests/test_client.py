import json

import httpx
import pytest

from veto import AsyncVeto, Veto, VetoError, VetoPending, VetoStopped


def scripted(responses):
    """A transport that answers from a script and records requests."""
    calls = []

    def handler(request: httpx.Request) -> httpx.Response:
        calls.append(request)
        status, body, headers = responses.pop(0)
        return httpx.Response(status, json=body, headers=headers or {})

    return httpx.MockTransport(handler), calls


def action(**over):
    base = {
        "id": "act_1",
        "status": "approved",
        "tool": "gmail.send",
        "decision": None,
        "grant": None,
        "hold_reasons": [],
        "options": [],
        "review_deadline": None,
        "received_at": "2026-01-01T00:00:00Z",
        "decided_at": None,
    }
    base.update(over)
    return base


def client(transport):
    return Veto("veto_sk_test", "https://veto.test/", http_client=httpx.Client(transport=transport), max_retries=2)


def test_submit_sends_key_wait_and_idempotency_key():
    transport, calls = scripted([(200, action(), None)])
    client(transport).submit("gmail.send", {"to": ["a@b.c"]}, request="send it", wait=10)
    req = calls[0]
    assert str(req.url) == "https://veto.test/v1/actions?wait=10"
    assert req.headers["Authorization"] == "Bearer veto_sk_test"
    body = json.loads(req.content)
    assert body["request"] == "send it" and len(body["idempotency_key"]) >= 8


def test_retries_keep_the_idempotency_key():
    transport, calls = scripted(
        [(503, {"error": {"code": "unavailable", "message": "busy"}}, {"Retry-After": "0"}), (200, action(), None)]
    )
    client(transport).submit("t", {})
    keys = [json.loads(c.content)["idempotency_key"] for c in calls]
    assert len(keys) == 2 and keys[0] == keys[1]


def test_client_errors_are_not_retried():
    transport, calls = scripted([(400, {"error": {"code": "invalid_request", "message": "tool is required"}, "request_id": "req_1"}, None)])
    with pytest.raises(VetoError) as info:
        client(transport).submit("", {})
    assert info.value.code == "invalid_request" and info.value.request_id == "req_1" and len(calls) == 1


def test_guard_runs_the_safer_call_and_reports_success():
    decision = {"option": "proceed_safer", "approved": True, "tool": "gmail.create_draft", "args": {"to": []}, "decided_by": "model", "reasons": [], "confidence": 0.9}
    transport, calls = scripted(
        [(200, action(decision=decision, grant={"token": "vg1.x.y", "expires_at": "x"}), None), (201, {}, None)]
    )
    ran = []
    out = client(transport).guard("gmail.send", {"to": ["x@rival.io"]}, lambda tool, args, grant: ran.append((tool, args, grant)) or "sent")
    assert out == "sent"
    assert ran == [("gmail.create_draft", {"to": []}, "vg1.x.y")]
    assert json.loads(calls[1].content) == {"status": "succeeded", "detail": None}


def test_guard_raises_when_stopped():
    decision = {"option": "stop", "approved": False, "decided_by": "policy", "reasons": [{"code": "policy_forbid", "detail": "no-refunds"}], "confidence": None}
    transport, _ = scripted([(200, action(status="denied", decision=decision), None)])
    with pytest.raises(VetoStopped, match="policy_forbid"):
        client(transport).guard("stripe.refund", {}, lambda *a: pytest.fail("a stopped call ran"))


def test_guard_raises_when_nobody_decides():
    pending = action(status="pending_review", review_url="https://veto.test/actions/act_1")
    transport, _ = scripted([(202, pending, None), (200, pending, None)])
    with pytest.raises(VetoPending) as info:
        client(transport).guard("github.merge", {}, lambda *a: "merged", timeout=0)
    assert info.value.action["id"] == "act_1"


def test_guard_reports_failure_and_reraises():
    decision = {"option": "proceed", "approved": True, "tool": "t", "args": {}, "decided_by": "model", "reasons": [], "confidence": 0.95}
    transport, calls = scripted([(200, action(decision=decision), None), (201, {}, None)])

    def boom(*_):
        raise RuntimeError("smtp down")

    with pytest.raises(RuntimeError, match="smtp down"):
        client(transport).guard("t", {}, boom)
    assert json.loads(calls[1].content) == {"status": "failed", "detail": {"error": "smtp down"}}


async def test_async_guard():
    decision = {"option": "proceed", "approved": True, "tool": "crm.read", "args": {"id": 1}, "decided_by": "policy", "reasons": [], "confidence": None}
    transport, _ = scripted([(200, action(decision=decision), None), (201, {}, None)])
    veto = AsyncVeto("veto_sk_test", "https://veto.test", http_client=httpx.AsyncClient(transport=transport))

    async def run(tool, args, grant):
        return (tool, args)

    assert await veto.guard("crm.read", {"id": 1}, run) == ("crm.read", {"id": 1})
    await veto.aclose()
