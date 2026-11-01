"""Async HTTP clients for the Kuberan API.

All calls go through one shared ``httpx.AsyncClient`` so handlers never block
the bot's event loop and connections to the API are pooled.
"""
import logging
from typing import Any, Dict, Optional

import httpx

logger = logging.getLogger(__name__)

# Generous enough for a slow API, short enough that a hung request cannot pin
# a handler forever.
DEFAULT_TIMEOUT = httpx.Timeout(10.0, connect=5.0)


class KuberanAPIClient:
    """Client for the Kuberan API's internal (bot-secret) endpoints.

    Owns the shared ``httpx.AsyncClient``; user-scoped clients created with
    :meth:`for_user` reuse it.
    """

    def __init__(self, base_url: str, internal_secret: str,
                 transport: Optional[httpx.AsyncBaseTransport] = None,
                 timeout: httpx.Timeout = DEFAULT_TIMEOUT):
        self.base_url = base_url.rstrip("/")
        self._internal_headers = {"X-Internal-Secret": internal_secret}
        self.http = httpx.AsyncClient(
            base_url=self.base_url, timeout=timeout, transport=transport
        )

    async def aclose(self) -> None:
        """Close the shared HTTP client (call on shutdown)."""
        await self.http.aclose()

    def for_user(self, auth_token: str) -> "UserAPIClient":
        """Return a user-scoped client sharing this client's connection pool."""
        return UserAPIClient(self.http, auth_token)

    async def resolve_user(self, telegram_user_id: int) -> Optional[Dict[str, Any]]:
        """Resolve a Telegram user ID to a Kuberan user with a JWT.

        Returns None when the Telegram account is not linked (404). Any other
        failure raises.
        """
        response = await self.http.get(
            f"/api/v1/internal/telegram/resolve/{telegram_user_id}",
            headers=self._internal_headers,
        )
        if response.status_code == 404:
            return None
        response.raise_for_status()
        # {user_id, email, auth_token, default_currency}
        return response.json()

    async def complete_link(self, link_code: str, telegram_user_id: int, username: str,
                            first_name: str, default_currency: str = "USD"):
        """Complete the linking process with optional default currency."""
        payload = {
            "link_code": link_code,
            "telegram_user_id": telegram_user_id,
            "telegram_username": username,
            "telegram_first_name": first_name,
        }
        if default_currency:
            payload["default_currency"] = default_currency

        response = await self.http.post(
            "/api/v1/internal/telegram/complete-link",
            json=payload,
            headers=self._internal_headers,
        )
        response.raise_for_status()
        return response.json()

    async def record_activity(self, telegram_user_id: int) -> None:
        """Record bot activity. Best effort: failures are logged, never raised."""
        try:
            await self.http.post(
                f"/api/v1/internal/telegram/activity/{telegram_user_id}",
                headers=self._internal_headers,
                timeout=5.0,
            )
        except httpx.HTTPError as e:
            logger.warning("Failed to record activity: %s", type(e).__name__)


class UserAPIClient:
    """API client for user-scoped operations (uses the user's JWT)."""

    # Batch endpoint for progress of all active budgets. If the backend falls
    # back to /budgets/overview on a Gin static/param routing conflict, change
    # this single constant to match.
    BUDGETS_PROGRESS_PATH = "/api/v1/budgets/progress"

    def __init__(self, http: httpx.AsyncClient, auth_token: str):
        self.http = http
        self.headers = {"Authorization": f"Bearer {auth_token}"}

    async def _get(self, path: str, params: Optional[Dict[str, Any]] = None) -> Any:
        response = await self.http.get(path, params=params, headers=self.headers)
        response.raise_for_status()
        return response.json()

    async def _post(self, path: str, payload: Dict[str, Any]) -> Any:
        response = await self.http.post(path, json=payload, headers=self.headers)
        response.raise_for_status()
        return response.json()

    async def get_accounts(self):
        """Get user accounts"""
        result = await self._get("/api/v1/accounts", {"page_size": 100})
        return result.get('data', [])

    async def get_profile(self) -> Dict[str, Any]:
        """Get the user's profile (includes default_account_id)"""
        result = await self._get("/api/v1/profile")
        return result.get('user', {})

    async def create_transaction(self, data: Dict[str, Any]):
        """Create a transaction"""
        return await self._post("/api/v1/transactions", data)

    async def get_budgets(self):
        """Get user budgets"""
        result = await self._get("/api/v1/budgets")
        return result.get('data', [])

    async def get_budget_progress(self, budget_id: str):
        """Get progress for a single budget (budget_id is a UUID string)."""
        result = await self._get(f"/api/v1/budgets/{budget_id}/progress")
        return result.get('progress', {})

    async def get_budgets_progress(self):
        """Get progress for all active budgets in a single batch call.

        Returns a list of BudgetProgress dicts (empty list if none).
        """
        result = await self._get(self.BUDGETS_PROGRESS_PATH)
        return result.get('progress', [])

    async def create_category(self, name: str, category_type: str,
                              description: str = "", icon: str = "",
                              color: str = "", parent_id: Optional[str] = None):
        """Create a new category with optional description, icon, color, and parent."""
        payload: Dict[str, Any] = {"name": name, "type": category_type}
        if description:
            payload["description"] = description
        if icon:
            payload["icon"] = icon
        if color:
            payload["color"] = color
        if parent_id:
            payload["parent_id"] = parent_id

        result = await self._post("/api/v1/categories", payload)
        return result.get('category', {})

    async def create_cash_account(self, name: str, currency: str = ""):
        """Create a new cash account with optional currency."""
        payload: Dict[str, Any] = {"name": name}
        if currency:
            payload["currency"] = currency
        result = await self._post("/api/v1/accounts/cash", payload)
        return result.get('account', {})

    async def get_categories(self):
        """Get user categories"""
        result = await self._get("/api/v1/categories", {"page_size": 100})
        return result.get('data', [])

    async def get_monthly_summary(self, months: int = 1):
        """Get monthly summary for the last N months."""
        result = await self._get("/api/v1/transactions/monthly-summary", {"months": months})
        return result.get('data', [])

    async def get_receivables_summary(self):
        """Get the summary of open receivables (money others owe the user).

        Returns a dict: {total_outstanding, by_kind, by_counterparty}.
        """
        return await self._get("/api/v1/receivables/summary")
