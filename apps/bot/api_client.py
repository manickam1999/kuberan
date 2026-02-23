import requests
from typing import Dict, Any, Optional
import logging

logger = logging.getLogger(__name__)

class KuberanAPIClient:
    """Client for communicating with Kuberan API (internal endpoints)"""

    def __init__(self, base_url: str, internal_secret: str):
        self.base_url = base_url
        self.internal_secret = internal_secret
        self.headers = {"X-Internal-Secret": internal_secret}

    def resolve_user(self, telegram_user_id: int) -> Optional[Dict[str, Any]]:
        """Resolve Telegram user ID to Kuberan user with JWT token"""
        try:
            response = requests.get(
                f"{self.base_url}/api/v1/internal/telegram/resolve/{telegram_user_id}",
                headers=self.headers,
                timeout=10
            )
            response.raise_for_status()
            return response.json()  # Returns: {user_id, email, auth_token, default_currency}
        except requests.HTTPError as e:
            if e.response.status_code == 404:
                return None
            logger.error(f"Failed to resolve user {telegram_user_id}: {e}")
            raise
        except Exception as e:
            logger.error(f"Error resolving user {telegram_user_id}: {e}")
            raise

    def complete_link(self, link_code: str, telegram_user_id: int, username: str,
                      first_name: str, default_currency: str = "USD"):
        """Complete the linking process with optional default currency."""
        try:
            payload = {
                "link_code": link_code,
                "telegram_user_id": telegram_user_id,
                "telegram_username": username,
                "telegram_first_name": first_name,
            }
            if default_currency:
                payload["default_currency"] = default_currency

            response = requests.post(
                f"{self.base_url}/api/v1/internal/telegram/complete-link",
                json=payload,
                headers=self.headers,
                timeout=10
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            logger.error(f"Failed to complete link for code {link_code}: {e}")
            raise

    def record_activity(self, telegram_user_id: int):
        """Record bot activity"""
        try:
            requests.post(
                f"{self.base_url}/api/v1/internal/telegram/activity/{telegram_user_id}",
                headers=self.headers,
                timeout=5
            )
        except Exception as e:
            logger.warning(f"Failed to record activity for {telegram_user_id}: {e}")


class UserAPIClient:
    """API client for user-scoped operations (uses JWT token)"""

    def __init__(self, base_url: str, auth_token: str):
        self.base_url = base_url
        self.headers = {"Authorization": f"Bearer {auth_token}"}

    def get_accounts(self):
        """Get user accounts"""
        response = requests.get(
            f"{self.base_url}/api/v1/accounts",
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        result = response.json()
        return result.get('data', [])

    def create_transaction(self, data: Dict[str, Any]):
        """Create a transaction"""
        response = requests.post(
            f"{self.base_url}/api/v1/transactions",
            json=data,
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        return response.json()

    def get_budgets(self):
        """Get user budgets"""
        response = requests.get(
            f"{self.base_url}/api/v1/budgets",
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        result = response.json()
        return result.get('data', [])

    def get_budget_progress(self, budget_id: int):
        """Get budget progress"""
        response = requests.get(
            f"{self.base_url}/api/v1/budgets/{budget_id}/progress",
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        result = response.json()
        return result.get('progress', {})

    def create_category(self, name: str, category_type: str,
                        description: str = "", icon: str = "",
                        color: str = "", parent_id: Optional[str] = None):
        """Create a new category with optional description, icon, color, and parent."""
        payload = {"name": name, "type": category_type}
        if description:
            payload["description"] = description
        if icon:
            payload["icon"] = icon
        if color:
            payload["color"] = color
        if parent_id:
            payload["parent_id"] = parent_id

        response = requests.post(
            f"{self.base_url}/api/v1/categories",
            json=payload,
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        return response.json().get('category', {})

    def create_cash_account(self, name: str, currency: str = ""):
        """Create a new cash account with optional currency."""
        payload = {"name": name}
        if currency:
            payload["currency"] = currency
        response = requests.post(
            f"{self.base_url}/api/v1/accounts/cash",
            json=payload,
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        return response.json().get('account', {})

    def get_categories(self):
        """Get user categories"""
        response = requests.get(
            f"{self.base_url}/api/v1/categories",
            params={"page_size": 100},
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        result = response.json()
        return result.get('data', [])

    def get_monthly_summary(self, months: int = 1):
        """Get monthly summary for the last N months."""
        response = requests.get(
            f"{self.base_url}/api/v1/transactions/monthly-summary",
            params={"months": months},
            headers=self.headers,
            timeout=10
        )
        response.raise_for_status()
        result = response.json()
        return result.get('data', [])
