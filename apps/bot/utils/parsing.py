from typing import Optional


def find_account_by_name(accounts: list, account_name: Optional[str] = None) -> Optional[int]:
    """
    Find account ID by name or return first account.

    Args:
        accounts: List of account objects
        account_name: Optional account name to search for

    Returns:
        Account ID or None
    """
    if not accounts:
        return None

    if account_name:
        # Case-insensitive search
        for acc in accounts:
            if acc.get('name', '').lower() == account_name.lower():
                return acc.get('id')

    # Return first active account as default
    for acc in accounts:
        if acc.get('is_active', True):
            return acc.get('id')

    # Fallback to first account
    return accounts[0].get('id')
