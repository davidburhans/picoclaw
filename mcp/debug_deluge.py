import logging
import httpx
import os
import sys

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("debug_deluge")

# Configuration (hardcoded for simplicity, adjust if needed)
DELUGE_URL = os.getenv("DELUGE_URL", "https://deluge.burhans.box.ca")
DELUGE_PASSWORD = os.getenv("DELUGE_PASSWORD", "4.Uc,.|g@m`7")  # Add your password if not in env

if not DELUGE_PASSWORD:
    if os.path.exists("config.json"):
        import json
        with open("config.json", "r") as f:
            cfg = json.load(f)
            DELUGE_PASSWORD = cfg.get("DELUGE_PASSWORD", "")
    
    if not DELUGE_PASSWORD:
        print("Error: DELUGE_PASSWORD env var not set.")
        # Try a default or just warn
        # print("Please set DELUGE_PASSWORD env var or create a config.json")

class DelugeClient:
    """Client for Deluge Web JSON-RPC."""
    def __init__(self, base_url: str, password: str):
        self.base_url = base_url.rstrip("/")
        self.password = password
        self.session_cookie = None
        self.req_id = 0

    def _get_headers(self):
        headers = {"Content-Type": "application/json"}
        if self.session_cookie:
            headers["Cookie"] = f"_session_id={self.session_cookie}"
        return headers

    def call(self, method: str, params) -> any:
        self.req_id += 1
        payload = {
            "method": method,
            "params": params,
            "id": self.req_id
        }
        
        url = f"{self.base_url}/json"
        try:
            # print(f"Calling {method} with params: {params}") # Reduced spam
            resp = httpx.post(url, json=payload, headers=self._get_headers(), timeout=10.0, verify=False)
            data = resp.json()
            
            if data.get("error"):
                print(f"Deluge RPC Error: {data['error']}")
            
            return data.get("result")
        except Exception as e:
            print(f"Deluge Call Error: {e}")
            return None

    def login(self):
        """Authenticates and sets the session cookie."""
        # Check current session validity
        if self.session_cookie:
             if self.call("auth.check_session", []):
                 return

        # Explicit login to capture cookie
        try:
            with httpx.Client(verify=False) as client:
                payload = {"method": "auth.login", "params": [self.password], "id": 1}
                resp = client.post(f"{self.base_url}/json", json=payload)
                if resp.status_code == 200:
                   data = resp.json()
                   if data.get("result") is True:
                       self.session_cookie = resp.cookies.get("_session_id")
                       print(f"Logged in successfully. Cookie: {self.session_cookie}")
                   else:
                       print(f"Login failed: {data}")
                else:
                    print(f"Login HTTP Error: {resp.status_code}")
        except Exception as e:
            print(f"Login Exception: {e}")

def run_tests():
    client = DelugeClient(DELUGE_URL, DELUGE_PASSWORD)
    client.login()

    if not client.session_cookie:
        print("Login failed, aborting tests.")
        return

    # 1. Check connection
    connected = client.call("web.connected", [])
    print(f"Web Connected: {connected}")
    
    if not connected:
        hosts = client.call("web.get_hosts", [])
        if hosts:
            for h in hosts:
                # h structure: [id, hostname, port, status] e.g. ["hash", "127.0.0.1", 58846, "Online"]
                print(f"Found Host: {h}")
                if h[3] in ["Online", "Connected"]:
                    client.call("web.connect", [h[0]])
                    print(f"Connected to {h[0]}")
                    break
        else:
            print("No hosts found.")

    # Re-check connected
    connected = client.call("web.connected", [])
    if not connected:
        print("Still not connected to daemon.")

    # Base keys for testing
    keys = ["name", "state", "progress"]

    print("\n--- Test 1: web.get_torrents_status([{}, keys]) ---")
    res1 = client.call("web.get_torrents_status", [{}, keys])
    print(f"Result Type: {type(res1)}")
    if isinstance(res1, dict):
        print(f"Count: {len(res1)}")
        print(f"Keys: {list(res1.keys())[:5]}")
    else:
        print(f"Data: {res1}")

    print("\n--- Test 2: web.get_torrents_status([None, keys]) ---")
    res2 = client.call("web.get_torrents_status", [None, keys])
    print(f"Result Type: {type(res2)}")
    if isinstance(res2, dict):
        print(f"Count: {len(res2)}")
    else:
        print(f"Data: {res2}")

    print("\n--- Test 3: core.get_torrents_status([{}, keys]) ---")
    res3 = client.call("core.get_torrents_status", [{}, keys])
    print(f"Result Type: {type(res3)}")
    if isinstance(res3, dict):
        print(f"Count: {len(res3)}")
    else:
        print(f"Data: {res3}")

    print("\n--- Test 4: web.update_ui([keys, {}]) ---")
    res4 = client.call("web.update_ui", [keys, {}])
    print(f"Result Type: {type(res4)}")
    if isinstance(res4, dict) and 'torrents' in res4:
         print(f"Count: {len(res4['torrents'])}")
    else:
        print(f"Data: {res4}")

if __name__ == "__main__":
    run_tests()
