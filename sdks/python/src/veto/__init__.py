"""Python client for Veto, the approval layer between AI agents and the actions they take.

Ask Veto before running a tool call, and run only what it approves::

    from veto import Veto

    veto = Veto()  # reads VETO_API_KEY and VETO_URL
    veto.guard("gmail.send", args, lambda tool, args, grant: send(tool, args), request=user_message)
"""

from .client import AsyncVeto, Veto
from .errors import VetoError, VetoPending, VetoStopped

__all__ = ["Veto", "AsyncVeto", "VetoError", "VetoPending", "VetoStopped"]
__version__ = "1.0.0"
