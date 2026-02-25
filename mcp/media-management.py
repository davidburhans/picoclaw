import os
import httpx
import json
import logging
from typing import List, Dict, Any, Optional, Union
from mcp.server.fastmcp import FastMCP

# Initialize the MCP Server
mcp = FastMCP("MediaHome")

# Configuration - Loaded from Environment Variables for Security
# If keys are missing, try loading from mounted config.json
def load_config():
    config = {
        "SONARR_URL": os.getenv("SONARR_URL", "https://sonarr.burhans.box.ca"),
        "SONARR_API_KEY": os.getenv("SONARR_API_KEY", ""),
        "RADARR_URL": os.getenv("RADARR_URL", "https://radarr.burhans.box.ca"),
        "RADARR_API_KEY": os.getenv("RADARR_API_KEY", ""),
        "DELUGE_URL": os.getenv("DELUGE_URL", "https://deluge.burhans.box.ca"),
        "DELUGE_PASSWORD": os.getenv("DELUGE_PASSWORD", ""),
    }

    # Check for missing keys and attempt to load from config.json
    if not config["SONARR_API_KEY"] or not config["RADARR_API_KEY"] or not config["DELUGE_PASSWORD"]:
        config_path = "/root/.picoclaw/config.json"
        
        # Check standard paths
        if not os.path.exists(config_path):
             # Fallback for dev environment or local testing
             local_path = os.path.join(os.getcwd(), "config", "config.json")
             if os.path.exists(local_path):
                 config_path = local_path
        
        if os.path.exists(config_path):
            try:
                with open(config_path, "r") as f:
                    data = json.load(f)
                    mcp_env = data.get("mcp", {}).get("media-management", {}).get("env", {})
                    
                    if not config["SONARR_API_KEY"] and mcp_env.get("SONARR_API_KEY"):
                        config["SONARR_API_KEY"] = mcp_env["SONARR_API_KEY"]
                    if not config["RADARR_API_KEY"] and mcp_env.get("RADARR_API_KEY"):
                        config["RADARR_API_KEY"] = mcp_env["RADARR_API_KEY"]
                    if not config["DELUGE_PASSWORD"] and mcp_env.get("DELUGE_PASSWORD"):
                        config["DELUGE_PASSWORD"] = mcp_env["DELUGE_PASSWORD"]
                        
                    logger.info(f"Loaded configuration from {config_path}")
            except Exception as e:
                logger.error(f"Failed to load config.json: {e}")
        else:
            logger.warning(f"Config file not found at {config_path}, proceeding with env vars only.")

    # Log loaded configuration (masked)
    safe_config = config.copy()
    for key in safe_config:
        if "KEY" in key or "PASSWORD" in key:
            val = safe_config[key]
            if val:
                # Show first 2 and last 2 chars if long enough, else just mask
                if len(val) > 4:
                    safe_config[key] = f"{val[:2]}...{val[-2:]} (len={len(val)})"
                else:
                    safe_config[key] = "***"
    
    logger.info(f"MCP Configuration Loaded: {safe_config}")

    return config

# Logger setup
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

CONFIG = load_config()



# --- Helper Classes ---

class ArrClient:
    """Base client for Sonarr and Radarr API interactions."""
    def __init__(self, base_url: str, api_key: str):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.headers = {"X-Api-Key": self.api_key}

    def get(self, endpoint: str, params: Dict = None) -> Any:
        url = f"{self.base_url}/api/v3{endpoint}"
        try:
            resp = httpx.get(url, headers=self.headers, params=params, timeout=10.0)
            resp.raise_for_status()
            return resp.json()
        except Exception as e:
            logger.error(f"Error GET {url}: {e}")
            return None

    def post(self, endpoint: str, data: Dict) -> Any:
        url = f"{self.base_url}/api/v3{endpoint}"
        try:
            resp = httpx.post(url, headers=self.headers, json=data, timeout=10.0)
            resp.raise_for_status()
            return resp.json()
        except httpx.HTTPStatusError as e:
            logger.error(f"HTTP Error POST {url}: {e.response.text}")
            raise ValueError(f"API Error: {e.response.text}")
        except Exception as e:
            logger.error(f"Error POST {url}: {e}")
            raise

    def delete(self, endpoint: str, params: Dict = None) -> Any:
        url = f"{self.base_url}/api/v3{endpoint}"
        try:
            resp = httpx.delete(url, headers=self.headers, params=params, timeout=10.0)
            resp.raise_for_status()
            if resp.status_code == 204:
                return {"status": "success"}
            return resp.json()
        except Exception as e:
            logger.error(f"Error DELETE {url}: {e}")
            return None

    def put(self, endpoint: str, data: Dict) -> Any:
        url = f"{self.base_url}/api/v3{endpoint}"
        try:
            resp = httpx.put(url, headers=self.headers, json=data, timeout=10.0)
            resp.raise_for_status()
            return resp.json()
        except Exception as e:
            logger.error(f"Error PUT {url}: {e}")
            raise

    def get_id_by_name(self, endpoint: str, name_key: str, target_name: str) -> Optional[int]:
        """Resolves a human-readable name to an internal ID."""
        items = self.get(endpoint)
        if not items:
            return None
        
        # Exact match first
        for item in items:
            if item.get(name_key, "").lower() == target_name.lower():
                return item["id"]
        
        # Partial match fallback
        for item in items:
            if target_name.lower() in item.get(name_key, "").lower():
                return item["id"]
        
        # Fallback to first item if strictly necessary, or return None
        return items[0]["id"] if items else None

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

    def call(self, method: str, params: List[Any]) -> Any:
        self.req_id += 1
        payload = {
            "method": method,
            "params": params,
            "id": self.req_id
        }
        
        url = f"{self.base_url}/json"
        try:
            resp = httpx.post(url, json=payload, headers=self._get_headers(), timeout=10.0, verify=False) # verify=False for self-signed
            data = resp.json()
            
            if data.get("error"):
                logger.error(f"Deluge RPC Error: {data['error']}")
                raise Exception(f"Deluge RPC Error: {data['error']}")
            
            # Deep debug log for torrent status calls
            if "get_torrents_status" in method:
                logger.debug(f"Deluge Raw Response ({method}): {str(data.get('result'))[:500]}...") # Log first 500 chars

            return data.get("result")
        except Exception as e:
            logger.error(f"Deluge Call Error ({method}): {e}")
            return None

    def login(self):
        """Authenticates and sets the session cookie."""
        if self.session_cookie:
             if self.call("auth.check_session", []):
                 return

        # Explicit login to capture cookie
        try:
            with httpx.Client(verify=False) as client:
                payload = {"method": "auth.login", "params": [self.password], "id": 1}
                resp = client.post(f"{self.base_url}/json", json=payload, timeout=10.0)
                if resp.status_code == 200:
                   data = resp.json()
                   if data.get("result") is True:
                       self.session_cookie = resp.cookies.get("_session_id")
                       logger.info("Deluge login successful")
                   else:
                       logger.error(f"Deluge login failed: {data}")
                else:
                    logger.error(f"Deluge login HTTP error: {resp.status_code}")
        except Exception as e:
            logger.error(f"Deluge login exception: {e}")

# Initialize Clients
sonarr = ArrClient(CONFIG["SONARR_URL"], CONFIG["SONARR_API_KEY"])
radarr = ArrClient(CONFIG["RADARR_URL"], CONFIG["RADARR_API_KEY"])
deluge = DelugeClient(CONFIG["DELUGE_URL"], CONFIG["DELUGE_PASSWORD"])

# --- MCP Tools ---

@mcp.tool()
def search_media_availability(query: str) -> str:
    """
    Search for a movie or TV show in Radarr and Sonarr to check if it exists 
    or to get details (IDs) for adding it.
    
    Args:
        query: The clean title of the media (e.g., "The Matrix", "Breaking Bad"). 
               Do not include quality or release group info.
        
    Returns:
        A formatted string describing found items in both services, including TVDB/TMDB IDs 
        required for adding new content.
    """
    results = []
    
    # Search Sonarr
    series = sonarr.get("/series/lookup", {"term": query})
    if series:
        results.append("\n--- TV Series (Sonarr) ---")
        for s in series[:3]: # Limit to top 3
            exists = s.get("id") is not None # If it has an ID, it's already in the library
            status = "In Library" if exists else "Not Monitored"
            results.append(f"Title: {s['title']} ({s.get('year')}) | TVDB ID: {s.get('tvdbId')} | Status: {status}")

    # Search Radarr
    movies = radarr.get("/movie/lookup", {"term": query})
    if movies:
        results.append("\n--- Movies (Radarr) ---")
        for m in movies[:3]:
            exists = m.get("id") is not None and m.get("id") > 0
            status = "In Library" if exists else "Available to Add"
            results.append(f"Title: {m['title']} ({m.get('year')}) | TMDB ID: {m.get('tmdbId')} | Status: {status}")
            
    if not results:
        return "No results found in Sonarr or Radarr."
        
    return "\n".join(results)

@mcp.tool()
def list_configuration_options(service: str) -> str:
    """
    Lists the valid Quality Profiles (e.g., "HD - 1080p") and Root Folders (e.g., "/tv/") for a service.
    CRITICAL: You MUST use this BEFORE calling add_movie or add_series to ensure you create 
    valid requests.
    
    Args:
        service: "sonarr" or "radarr".
    """
    client = sonarr if service.lower() == "sonarr" else radarr
    
    profiles = client.get("/qualityprofile")
    folders = client.get("/rootfolder")
    
    out = [f"--- {service.upper()} Options ---"]
    
    if profiles:
        out.append("Quality Profiles:")
        for p in profiles:
            out.append(f" - {p['name']} (ID: {p['id']})")
            
    if folders:
        out.append("Root Folders:")
        for f in folders:
            out.append(f" - {f['path']} (Free: {f.get('freeSpace', 0) // 1024 // 1024 // 1024} GB)")
            
    return "\n".join(out)

@mcp.tool()
def add_movie(
    title: str, 
    tmdb_id: int, 
    quality_profile: str = "Any", 
    root_folder: str = "/movies/"
) -> str:
    """
    Adds a movie to Radarr and triggers an immediate search.
    
    Args:
        title: The title of the movie.
        tmdb_id: The TMDB ID (from search_media_availability).
        quality_profile: Exact name from list_configuration_options (e.g., "HD - 1080p").
        root_folder: Exact path from list_configuration_options (e.g., "/movies/").
    """
    # Resolve Profile ID
    q_id = radarr.get_id_by_name("/qualityprofile", "name", quality_profile)
    if not q_id:
        return f"Error: Could not find quality profile matching '{quality_profile}'"

    # Verify Root Folder (Simple check if the path string exists in the config)
    # We trust the user/agent provided a valid one, or we use the API to validate.
    # For robustness, we just use what is passed, assuming list_options was used.

    payload = {
        "title": title,
        "tmdbId": tmdb_id,
        "qualityProfileId": q_id,
        "rootFolderPath": root_folder,
        "monitored": True,
        "minimumAvailability": "released", # Default to released to avoid cams
        "addOptions": {
            "searchForMovie": True
        }
    }
    
    try:
        radarr.post("/movie", payload)
        return f"Successfully added '{title}' (TMDB: {tmdb_id}) to Radarr and started search."
    except Exception as e:
        return f"Failed to add movie: {str(e)}"

@mcp.tool()
def add_series(
    title: str,
    tvdb_id: int,
    quality_profile: str = "Any",
    root_folder: str = "/tv/"
) -> str:
    """
    Adds a TV Series to Sonarr and triggers a search for missing episodes.
    
    Args:
        title: The title of the series.
        tvdb_id: The TVDB ID (from search_media_availability).
        quality_profile: Exact name from list_configuration_options (e.g., "HD - 1080p").
        root_folder: Exact path from list_configuration_options (e.g., "/tv/").
    """
    q_id = sonarr.get_id_by_name("/qualityprofile", "name", quality_profile)
    if not q_id:
        return f"Error: Could not find quality profile matching '{quality_profile}'"

    # Get Language Profile (Default to English/First available)
    lang_profiles = sonarr.get("/languageprofile")
    l_id = lang_profiles[0]["id"] if lang_profiles else 1

    payload = {
        "title": title,
        "tvdbId": tvdb_id,
        "qualityProfileId": q_id,
        "languageProfileId": l_id,
        "rootFolderPath": root_folder,
        "monitored": True,
        "seasonFolder": True,
        "addOptions": {
            "searchForMissingEpisodes": True
        }
    }

    try:
        sonarr.post("/series", payload)
        return f"Successfully added '{title}' (TVDB: {tvdb_id}) to Sonarr and started search."
    except Exception as e:
        return f"Failed to add series: {str(e)}"

@mcp.tool()
def get_download_status() -> str:
    """
    Checks the Deluge download client for active torrents and cross-references them 
    with Sonarr/Radarr queues. Use this to get a high-level overview of what is currently 
    downloading.
    """
    status_report = []
    
    # 1. Check Deluge
    try:
        deluge.login()
        
        # Check connection status
        connected = deluge.call("web.connected", [])
        if not connected:
            # Try to connect to the first available host
            hosts = deluge.call("web.get_hosts", [])
            if hosts:
                # hosts is a list of [id, host, port, status]
                # status: "Offline", "Online", "Connected"
                online_hosts = [h for h in hosts if h[3] != "Offline"]
                if online_hosts:
                    deluge.call("web.connect", [online_hosts[0][0]])
                    status_report.append(f"(Debug: Connected to Deluge daemon {online_hosts[0][0]})")
                else:
                    status_report.append("Error: No online Deluge daemons found.")
            else:
                status_report.append("Error: No Deluge hosts configured.")
        
        # "state", "name", "progress", "download_payload_rate", "eta"
        # Using web.update_ui as it's more comprehensive and robust in the Web API
        keys = ["name", "state", "progress", "download_payload_rate", "eta"]
        ui_data = deluge.call("web.update_ui", [keys, {}])
        
        if ui_data and isinstance(ui_data, dict) and 'torrents' in ui_data:
            torrents = ui_data['torrents']
            status_report.append("\n--- Deluge Active Downloads ---")
            state_counts = {}
            active_count = 0
            
            for t_id, t_data in torrents.items():
                state = t_data.get('state', 'Unknown')
                state_counts[state] = state_counts.get(state, 0) + 1
                
                if state.lower() in ['downloading', 'seeding', 'queued', 'checking', 'allocating', 'moving', 'error']:
                    name = t_data.get('name', 'Unknown')
                    progress = f"{t_data.get('progress', 0):.1f}%"
                    speed_val = t_data.get('download_payload_rate', 0)
                    speed = f"{speed_val / 1024:.1f} KiB/s" if speed_val else "0 KiB/s"
                    
                    status_report.append(f"[{state}] {name} (ID: {t_id}) - {progress} @ {speed}")
                    active_count += 1
            
            if active_count == 0:
                status_report.append("No active downloads (filtering applied).")
                
            status_report.append(f"\n(Debug: Total {len(torrents)} items. States: {state_counts})")
        else:
            status_report.append("\n--- Deluge: No torrents found or API error ---")
            
    except Exception as e:
        logger.error(f"Error checking Deluge: {str(e)}")
        status_report.append(f"Error checking Deluge: {str(e)}")



    # 2. Check Sonarr Queue
    try:
        resp = sonarr.get("/queue")
        items = resp.get("records", []) if isinstance(resp, dict) else []
        if items:
            status_report.append("\n--- Sonarr Queue ---")
            for item in items:
                # Try multiple fields for title
                title = item.get("title") or item.get("series", {}).get("title") or "Unknown"
                status = item.get("status", "Unknown")
                timeleft = item.get("timeleft", "Unknown")
                q_id = item.get("id", "Unknown")
                status_report.append(f"{title} (ID: {q_id}): {status} (Timeleft: {timeleft})")
    except Exception as e:
        logger.error(f"Error checking Sonarr queue: {e}")

    # 3. Check Radarr Queue
    try:
        resp = radarr.get("/queue")
        items = resp.get("records", []) if isinstance(resp, dict) else []
        if items:
            status_report.append("\n--- Radarr Queue ---")
            for item in items:
                # Try multiple fields for title
                title = item.get("title") or item.get("movie", {}).get("title") or "Unknown"
                status = item.get("status", "Unknown")
                timeleft = item.get("timeleft", "Unknown")
                q_id = item.get("id", "Unknown")
                status_report.append(f"{title} (ID: {q_id}): {status} (Timeleft: {timeleft})")
    except Exception as e:
        logger.error(f"Error checking Radarr queue: {e}")
        
    final_report = "\n".join(status_report)
    logger.debug(f"Final Status Report: {final_report}")
    return final_report

@mcp.tool()
def get_detailed_status(service: str) -> str:
    """
    Get comprehensive status from Sonarr or Radarr.
    returns:
    1. Queue messages (why items are warning/failed).
    2. Indexer status (are trackers down?).
    3. Disk space (is the drive full?).
    Use this to diagnose why downloads are not starting or are stuck.
    
    Args:
        service: "sonarr" or "radarr".
    """
    client = sonarr if service.lower() == "sonarr" else radarr
    report = [f"--- Detailed {service.upper()} Status ---"]
    
    # 1. Queue Metadata
    queue = client.get("/queue")
    items = queue.get("records", []) if isinstance(queue, dict) else []
    if items:
        report.append("\nQueue Health:")
        for item in items:
            title = item.get("series", {}).get("title") or item.get("movie", {}).get("title") or "Unknown"
            status = item.get("status")
            tracked_status = item.get("trackedDownloadStatus", "N/A")
            messages = ", ".join([msg.get("message", "") for msg in item.get("statusMessages", [])])
            q_id = item.get("id", "Unknown")
            report.append(f" - {title} (ID: {q_id}): [{status}] Tracked: {tracked_status} | Msgs: {messages}")
    else:
        report.append("\nQueue is empty.")

    # 2. Indexer Status
    indexers = client.get("/indexerstatus")
    if indexers:
        report.append("\nIndexer Status:")
        for idx in indexers:
            name = idx.get("name", "Unknown")
            status = "OK" if not idx.get("initialFailure") else "FAILING"
            report.append(f" - {name}: {status}")
            
    # 3. Disk Space
    disk = client.get("/diskspace")
    if disk:
        report.append("\nDisk Space:")
        for d in disk:
            label = d.get("label", d.get("path"))
            free = d.get("freeSpace", 0) // (1024**3)
            total = d.get("totalSpace", 0) // (1024**3)
            report.append(f" - {label}: {free}GB free / {total}GB total")
            
    return "\n".join(report)

@mcp.tool()
def find_stuck_media(service: str) -> str:
    """
    Identifies items in the queue that require intervention.
    Criteria for "stuck":
    - Status is "Warning", "Error", "Stalled", or "Failed".
    - Has error messages attached.
    
    Args:
        service: "sonarr" or "radarr".
    """
    client = sonarr if service.lower() == "sonarr" else radarr
    queue = client.get("/queue")
    items = queue.get("records", []) if isinstance(queue, dict) else []
    
    stuck_items = []
    for item in items:
        # Check for common "stuck" indicators
        tracked_status = item.get("trackedDownloadStatus", "")
        status = item.get("status", "")
        messages = item.get("statusMessages", [])
        
        is_stuck = (
            tracked_status.lower() in ["warning", "error"] or
            status.lower() in ["stalled", "failed"] or
            len(messages) > 0
        )
        
        if is_stuck:
            title = item.get("series", {}).get("title") or item.get("movie", {}).get("title") or "Unknown"
            msg_text = ", ".join([m.get("message", "") for m in messages])
            q_id = item.get("id")
            stuck_items.append(f" - ID {q_id}: {title} | Status: {status} | Issue: {msg_text}")

    if not stuck_items:
        return f"No stuck items found in {service} queue."
        
    out = [f"--- Found {len(stuck_items)} stuck items in {service} ---"]
    out.extend(stuck_items)
    out.append("\nRecommendation: Call manage_queue_item(service, ID, ...) to resolve.")
    return "\n".join(out)

@mcp.tool()
def retry_stuck_download(service: str, queue_id: int) -> str:
    """
    Legacy helper. Equivalent to manage_queue_item(..., remove_from_client=True, blocklist=True, skip_redownload=False).
    Use this for a "standard retry" of a bad release.
    
    Args:
        service: "sonarr" or "radarr".
        queue_id: The queue ID (from get_detailed_status).
    """
    client = sonarr if service.lower() == "sonarr" else radarr
    
    # 1. Get info before deleting to trigger search later
    queue = client.get("/queue")
    records = queue.get("records", []) if isinstance(queue, dict) else []
    target_item = next((r for r in records if r["id"] == queue_id), None)
    
    if not target_item:
        return f"Error: Could not find item with ID {queue_id} in {service} queue."

    # 2. Delete and Blocklist
    client.delete(f"/queue/{queue_id}", {"removeFromClient": "true", "blocklist": "true"})
    
    # 3. Trigger Search
    if service.lower() == "sonarr":
        series_id = target_item.get("seriesId")
        if series_id:
            client.post("/command", {"name": "SeriesSearch", "seriesId": series_id})
    else:
        movie_id = target_item.get("movieId")
        if movie_id:
            client.post("/command", {"name": "MovieSearch", "movieIds": [movie_id]})
            
    return f"Successfully removed item {queue_id} from {service} queue (blocklisted) and triggered a new search."

@mcp.tool()
def manual_release_search(service: str, media_id: int) -> str:
    """
    Search for alternative releases for a specific media item.
    Use this if the automated search is failing to find a valid release.
    
    Args:
        service: "sonarr" or "radarr".
        media_id: The internal database ID of the Series/Movie (NOT the queue ID).
    """
    client = sonarr if service.lower() == "sonarr" else radarr
    endpoint = f"/release?seriesId={media_id}" if service.lower() == "sonarr" else f"/release?movieId={media_id}"
    
    releases = client.get(endpoint)
    if not releases:
        return f"No releases found for {service} ID {media_id}."
        
    out = [f"--- Available Releases for {service.upper()} ID {media_id} ---"]
    for r in releases[:10]: # Top 10
        title = r.get("title")
        size = r.get("size", 0) // (1024**2) # MB
        seeders = r.get("seeders", 0)
        quality = r.get("quality", {}).get("quality", {}).get("name", "Unknown")
        rejected = " [REJECTED]" if not r.get("approved") else ""
        out.append(f" - {title} | {quality} | {size}MB | Seeds: {seeders}{rejected}")
        if not r.get("approved"):
            reasons = ", ".join(r.get("rejections", []))
            out.append(f"   Reason: {reasons}")
            
    return "\n".join(out)

@mcp.tool()
def manage_deluge_torrent(torrent_id: str, action: str) -> str:
    """
    Direct interface to the Deluge download client.
    
    Args:
        torrent_id: The hash string.
        action: "pause" (stop downloading), "resume" (start), "remove" (delete torrent AND data), "recheck" (verify hash).
    """
    try:
        deluge.login()
        if action == "pause":
            deluge.call("core.pause_torrent", [[torrent_id]])
        elif action == "resume":
            deluge.call("core.resume_torrent", [[torrent_id]])
        elif action == "remove":
            deluge.call("core.remove_torrent", [torrent_id, True]) # True = remove data
        elif action == "recheck":
            deluge.call("core.force_recheck", [[torrent_id]])
        else:
            return f"Error: Unknown action '{action}'"
            
        return f"Successfully executed '{action}' on torrent {torrent_id}"
    except Exception as e:
        return f"Error managing Deluge torrent: {str(e)}"

@mcp.tool()
def get_torrent_info(torrent_id: str) -> str:
    """
    Get technical details about a specific torrent in Deluge.
    Returns:
    - Save Path (Server-side path).
    - File list (Are the files actually there?).
    - Tracker status (Is it actually downloading?).
    
    Args:
        torrent_id: The hash string.
    """
    try:
        deluge.login()
        # timestamp, total_done, total_payload_download, total_uploaded, download_payload_rate, upload_payload_rate, eta, ratio, distributed_copies, is_auto_managed, time_added, tracker_host, save_path, total_size, num_files, message, last_seen_complete
        # We need: name, save_path, files, progress, state, tracker_status
        keys = ["name", "save_path", "files", "progress", "state", "total_size", "message", "tracker_status"]
        status = deluge.call("core.get_torrent_status", [torrent_id, keys])
        
        if not status:
            return f"Error: Torrent ID {torrent_id} not found."
            
        out = [f"--- Torrent Info: {status.get('name')} ---"]
        out.append(f"ID: {torrent_id}")
        out.append(f"State: {status.get('state')}")
        out.append(f"Progress: {status.get('progress', 0):.2f}%")
        out.append(f"Save Path: {status.get('save_path')}")
        out.append(f"Total Size: {status.get('total_size', 0) / (1024**3):.2f} GB")
        out.append(f"Message: {status.get('message', 'None')}")
        out.append(f"Tracker Status: {status.get('tracker_status', 'Unknown')}")
        
        files = status.get("files", [])
        if files:
            out.append(f"\nFiles ({len(files)}):")
            # Sort by size desc, show top 10
            sorted_files = sorted(files, key=lambda x: x.get("size", 0), reverse=True)
            for f in sorted_files[:10]:
                f_path = f.get("path")
                f_size = f.get("size", 0) / (1024**2)
                f_prog = f.get("progress", 0) * 100
                out.append(f" - {f_path} ({f_size:.1f} MB) [{f_prog:.0f}%]")
            if len(files) > 10:
                out.append(f" ... and {len(files) - 10} more.")
                
        return "\n".join(out)

    except Exception as e:
        return f"Error getting torrent info: {str(e)}"

@mcp.tool()
def manage_queue_item(
    service: str, 
    queue_id: int, 
    remove_from_client: bool = True, 
    blocklist: bool = False,
    skip_redownload: bool = False
) -> str:
    """
    Advanced queue management tool.
    
    Args:
        service: "sonarr" or "radarr"
        queue_id: The ID from get_detailed_status.
        remove_from_client: CAUTION. True = Delete data. False = Keep data (use if importing manually).
        blocklist: True = This release is bad, never grab it again. False = This release is fine, just stuck.
        skip_redownload: True = Just delete it. False = Delete and search for a replacement immediately.
    """
    client = sonarr if service.lower() == "sonarr" else radarr
    
    # 1. Get info to trigger search later (if needed)
    queue = client.get("/queue")
    records = queue.get("records", []) if isinstance(queue, dict) else []
    target_item = next((r for r in records if r["id"] == queue_id), None)
    
    if not target_item:
        return f"Error: Could not find item with ID {queue_id} in {service} queue."

    # 2. Delete
    # API: DELETE /queue/{id}?removeFromClient={bool}&blocklist={bool}&skipRedownload={bool} (skipRedownload might vary by version, usually implicit if not searching)
    # Actually, skipRedownload isn't a direct param on delete usually, it's just about whether we trigger a search AFTER.
    # Blocklist is a param. RemoveFromClient is a param.
    
    params = {
        "removeFromClient": str(remove_from_client).lower(),
        "blocklist": str(blocklist).lower()
    }
    
    try:
        client.delete(f"/queue/{queue_id}", params)
        msg = [f"Successfully removed item {queue_id} from {service} queue."]
        if remove_from_client:
            msg.append("- Removed from download client.")
        else:
            msg.append("- Kept in download client (files preserved).")
            
        if blocklist:
            msg.append("- Blocklisted release.")
            
        # 3. Trigger Search (if not skipped)
        if not skip_redownload:
            if service.lower() == "sonarr":
                series_id = target_item.get("seriesId")
                if series_id:
                    client.post("/command", {"name": "SeriesSearch", "seriesId": series_id})
                    msg.append("- Triggered SeriesSearch.")
            else:
                movie_id = target_item.get("movieId")
                if movie_id:
                    client.post("/command", {"name": "MovieSearch", "movieIds": [movie_id]})
                    msg.append("- Triggered MovieSearch.")
        else:
            msg.append("- Skipped redownload/search.")
            
        return "\n".join(msg)
        
    except Exception as e:
        return f"Failed to remove queue item: {str(e)}"

@mcp.tool()
def trigger_manual_import(service: str, download_id: str = None, path: str = None) -> str:
    """
    Forces Sonarr/Radarr to scan a specific path or download ID for importable files.
    Use this when a download is 100% complete in Deluge but stuck in "Downloading" state in the Arr.
    
    Args:
        service: "sonarr" or "radarr".
        download_id: The hash/ID from the download client (optional).
        path: The SERVER-SIDE path to scan (optional). 
              CRITICAL: Verify this path exists using get_torrent_info first.
    """
    if not download_id and not path:
        return "Error: Must provide either download_id or path."
        
    client = sonarr if service.lower() == "sonarr" else radarr
    command_name = "DownloadedEpisodesScan" if service.lower() == "sonarr" else "DownloadedMovieScan"
    
    payload = {"name": command_name}
    if download_id:
        payload["downloadClientId"] = download_id
    if path:
        payload["path"] = path
        # Specific quirk: Radarr/Sonarr often require 'importMode': 'Move' or 'Copy' or 'Auto'
        # Default is usually Auto.
        
    try:
        client.post("/command", payload)
        details = f"ID: {download_id}" if download_id else f"Path: {path}"
        return f"Triggered {command_name} for {details}."
    except Exception as e:
        return f"Failed to trigger import: {str(e)}"

@mcp.tool()
def list_library_contents(service: str, limit: int = 20) -> str:
    """
    Lists the contents of the Sonarr (Series) or Radarr (Movies) library.
    Use this to see what is currently being monitored/downloaded.
    
    Args:
        service: "sonarr" or "radarr".
        limit: Number of items to return (default 20, max 50).
    """
    client = sonarr if service.lower() == "sonarr" else radarr
    endpoint = "/series" if service.lower() == "sonarr" else "/movie"
    
    items = client.get(endpoint)
    if not items:
        return f"No items found in {service} library."
        
    # Sort by added date descending to show newest first
    items.sort(key=lambda x: x.get("added", ""), reverse=True)
    
    # Cap limit
    limit = min(limit, 50)
    
    out = [f"--- {service.upper()} Library (Top {limit} Newest) ---"]
    
    for item in items[:limit]:
        title = item.get("title", "Unknown")
        year = item.get("year", "????")
        monitored = "Monitored" if item.get("monitored") else "Unmonitored"
        
        # Sonarr specifics
        if service.lower() == "sonarr":
            season_count = item.get("seasonCount", 0)
            status = item.get("status", "Unknown") # continuing, ended
            out.append(f" - {title} ({year}) | {status} | {season_count} Seasons | {monitored}")
        else:
            # Radarr specifics
            downloaded = "Downloaded" if item.get("hasFile") else "Missing"
            status = item.get("status", "Unknown") # released, announced
            out.append(f" - {title} ({year}) | {status} | {downloaded} | {monitored}")
            
    return "\n".join(out)

if __name__ == "__main__":
    # Check for keys
    if not CONFIG["SONARR_API_KEY"] or not CONFIG["RADARR_API_KEY"]:
        print("WARNING: API Keys for Sonarr/Radarr not set in environment variables.")
        print("Please export SONARR_API_KEY and RADARR_API_KEY.")
    
    mcp.run()
