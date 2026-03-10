import requests

WATCHDAWG_BASE = "http://watchdawg:9091"


class HistoryAPI:
    """Thin wrapper around the WatchDawg history REST API."""

    def get_check(self, check_name: str, limit: int = 100) -> list[dict]:
        """GET /history/{check_name} — returns records list or empty list on 404."""
        resp = requests.get(
            f"{WATCHDAWG_BASE}/history/{check_name}",
            params={"limit": limit},
            timeout=5,
        )
        if resp.status_code == 404:
            return []
        resp.raise_for_status()
        return resp.json()["checks"].get(check_name, [])

    def get_check_raw(self, check_name: str, limit: int = 100) -> requests.Response:
        """GET /history/{check_name} — returns raw response (for status code assertions)."""
        return requests.get(
            f"{WATCHDAWG_BASE}/history/{check_name}",
            params={"limit": limit},
            timeout=5,
        )

    def get_all(self, limit: int = 100) -> dict[str, list[dict]]:
        """GET /history/* — returns {check_name: [records]} map."""
        resp = requests.get(
            f"{WATCHDAWG_BASE}/history/*",
            params={"limit": limit},
            timeout=5,
        )
        resp.raise_for_status()
        return resp.json()["checks"]
