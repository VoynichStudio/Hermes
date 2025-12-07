#!/usr/bin/env python3
"""
ClickUp Project Setup Script for Hermes Chat Engine
Creates Epics, Stories, and Tasks for MMORPG Chat Engine development.

Usage:
    python clickup_setup.py --api-key YOUR_API_KEY
    python clickup_setup.py --api-key YOUR_API_KEY --test  # Test API connection
    python clickup_setup.py --api-key YOUR_API_KEY --team-id 12345  # Skip workspace lookup

Requirements:
    pip install requests
"""

import argparse
import requests
import time
import sys
from typing import Optional
from dataclasses import dataclass, field

# ============================================================================
# Configuration
# ============================================================================

WORKSPACE_NAME = "Voynich Studios"
SPACE_NAME = "Core"
FOLDER_NAME = "Hermes"
LIST_NAME = "Product Backlog"

BASE_URL = "https://api.clickup.com/api/v2"

# Enable debug mode for verbose output
DEBUG = False

# Jira-like statuses for Scrum
STATUSES = [
    {"status": "backlog", "color": "#87909e", "orderindex": 0},
    {"status": "to do", "color": "#d3d3d3", "orderindex": 1},
    {"status": "in progress", "color": "#4194f6", "orderindex": 2},
    {"status": "in review", "color": "#a875ff", "orderindex": 3},
    {"status": "done", "color": "#6bc950", "orderindex": 4},
]

# ============================================================================
# Data Classes
# ============================================================================

@dataclass
class Task:
    """Represents a sub-task (actual work item)"""
    name: str
    description: str = ""

@dataclass
class Story:
    """Represents a Story with its tasks"""
    name: str
    description: str = ""
    priority: int = 3  # 1=Urgent, 2=High, 3=Normal, 4=Low
    tasks: list[Task] = field(default_factory=list)

@dataclass
class Epic:
    """Represents an Epic with its stories"""
    name: str
    description: str
    priority: int = 2  # Epics are usually High priority
    stories: list[Story] = field(default_factory=list)
    tags: list[str] = field(default_factory=list)

# ============================================================================
# API Client
# ============================================================================

class ClickUpClient:
    def __init__(self, api_key: str):
        self.api_key = api_key
        self.headers = {
            "Authorization": api_key,
            "Content-Type": "application/json"
        }
        self.request_count = 0

    def _request(self, method: str, endpoint: str, data: dict = None) -> dict:
        """Make an API request with rate limiting"""
        self.request_count += 1

        # Rate limiting: ClickUp allows 100 requests per minute
        if self.request_count % 10 == 0:
            time.sleep(0.5)  # Small delay every 10 requests

        url = f"{BASE_URL}/{endpoint}"

        if DEBUG:
            print(f"  DEBUG: {method} {url}")
            print(f"  DEBUG: Headers: {self.headers}")
            if data:
                print(f"  DEBUG: Data: {data}")

        try:
            if method == "GET":
                response = requests.get(url, headers=self.headers)
            elif method == "POST":
                response = requests.post(url, headers=self.headers, json=data)
            elif method == "PUT":
                response = requests.put(url, headers=self.headers, json=data)
            else:
                raise ValueError(f"Unknown method: {method}")

            if DEBUG:
                print(f"  DEBUG: Status: {response.status_code}")
                print(f"  DEBUG: Response: {response.text[:500] if response.text else 'empty'}")

            if response.status_code == 429:  # Rate limited
                print("  Rate limited, waiting 60 seconds...")
                time.sleep(60)
                return self._request(method, endpoint, data)

            response.raise_for_status()
            return response.json() if response.text else {}

        except requests.exceptions.RequestException as e:
            print(f"  Error: {e}")
            if hasattr(e, 'response') and e.response is not None:
                print(f"  Response: {e.response.text}")
            raise

    def test_connection(self) -> dict:
        """Test API connection and return diagnostic info"""
        results = {
            "api_key_format": "valid" if self.api_key.startswith("pk_") else "invalid (should start with pk_)",
            "endpoints_tested": []
        }

        # Test different endpoint variations
        endpoints_to_test = [
            ("GET", "user", "Get current user"),
            ("GET", "team", "Get teams/workspaces"),
            ("GET", "team/", "Get teams (with trailing slash)"),
        ]

        for method, endpoint, description in endpoints_to_test:
            url = f"{BASE_URL}/{endpoint}"
            try:
                response = requests.request(method, url, headers=self.headers)
                results["endpoints_tested"].append({
                    "endpoint": endpoint,
                    "description": description,
                    "status": response.status_code,
                    "success": response.status_code == 200,
                    "response": response.text[:200] if response.text else "empty"
                })
            except Exception as e:
                results["endpoints_tested"].append({
                    "endpoint": endpoint,
                    "description": description,
                    "status": "error",
                    "success": False,
                    "response": str(e)
                })

        return results

    def get_teams(self) -> list:
        """Get all workspaces/teams"""
        return self._request("GET", "team")["teams"]

    def get_user(self) -> dict:
        """Get current authenticated user"""
        return self._request("GET", "user")

    def get_spaces(self, team_id: str) -> list:
        """Get all spaces in a workspace"""
        return self._request("GET", f"team/{team_id}/space")["spaces"]

    def create_space(self, team_id: str, name: str) -> dict:
        """Create a new space"""
        data = {
            "name": name,
            "multiple_assignees": True,
            "features": {
                "due_dates": {"enabled": True, "start_date": True, "remap_due_dates": True},
                "time_tracking": {"enabled": True},
                "tags": {"enabled": True},
                "time_estimates": {"enabled": True},
                "checklists": {"enabled": True},
                "custom_fields": {"enabled": True},
                "dependency_warning": {"enabled": True},
                "portfolios": {"enabled": True},
                "milestones": {"enabled": True}
            }
        }
        return self._request("POST", f"team/{team_id}/space", data)

    def get_folders(self, space_id: str) -> list:
        """Get all folders in a space"""
        return self._request("GET", f"space/{space_id}/folder")["folders"]

    def create_folder(self, space_id: str, name: str) -> dict:
        """Create a new folder"""
        return self._request("POST", f"space/{space_id}/folder", {"name": name})

    def get_lists(self, folder_id: str) -> list:
        """Get all lists in a folder"""
        return self._request("GET", f"folder/{folder_id}/list")["lists"]

    def create_list(self, folder_id: str, name: str) -> dict:
        """Create a new list with Scrum statuses"""
        data = {
            "name": name,
            "status": "backlog"
        }
        return self._request("POST", f"folder/{folder_id}/list", data)

    def create_task(self, list_id: str, name: str, description: str = "",
                    priority: int = 3, tags: list = None, parent: str = None) -> dict:
        """Create a task or subtask"""
        data = {
            "name": name,
            "description": description,
            "priority": priority,
            "status": "backlog",
        }
        if tags:
            data["tags"] = tags
        if parent:
            data["parent"] = parent

        return self._request("POST", f"list/{list_id}/task", data)

    def create_checklist(self, task_id: str, name: str) -> dict:
        """Create a checklist on a task"""
        return self._request("POST", f"task/{task_id}/checklist", {"name": name})

    def create_checklist_item(self, checklist_id: str, name: str) -> dict:
        """Add an item to a checklist"""
        return self._request("POST", f"checklist/{checklist_id}/checklist_item", {"name": name})

    def create_tag(self, space_id: str, name: str, bg_color: str = "#4169E1") -> dict:
        """Create a tag in a space"""
        data = {
            "tag": {
                "name": name,
                "tag_bg": bg_color
            }
        }
        return self._request("POST", f"space/{space_id}/tag", data)

# ============================================================================
# Epic Data - All your implementation plan
# ============================================================================

def get_epics() -> list[Epic]:
    """Returns all Epics with their Stories and Tasks"""

    epics = []

    # -------------------------------------------------------------------------
    # Epic 0: Foundation & Critical Fixes
    # -------------------------------------------------------------------------
    epic0 = Epic(
        name="Epic 0: Foundation & Critical Fixes",
        description="Stabilize the current codebase and establish proper development practices",
        priority=1,  # Urgent
        tags=["foundation", "p0-blocker"]
    )

    # Story 0.1
    story_0_1 = Story(
        name="Story 0.1: Fix Critical Bugs",
        description="Fix all critical bugs in the current implementation",
        priority=1,
        tasks=[
            Task("Fix double RUnlock() in SendMessage()", "No panic on concurrent message sends"),
            Task("Fix race condition in channel user access", "Hold read lock while iterating ch.Users"),
            Task("Add proper error handling in main()", "Server logs fatal error if port binding fails"),
            Task("Add graceful shutdown with signal handling", "SIGTERM/SIGINT triggers clean connection drain"),
        ]
    )
    epic0.stories.append(story_0_1)

    # Story 0.2
    story_0_2 = Story(
        name="Story 0.2: Configuration Management",
        description="Add proper configuration support with environment variables and config files",
        priority=1,
        tasks=[
            Task("Create config struct with all settings", "Single source of truth for configuration"),
            Task("Add environment variable support", "All settings configurable via env vars"),
            Task("Add config file support (YAML/TOML)", "Optional config file with env override"),
            Task("Validate configuration on startup", "Server fails fast with clear error on bad config"),
            Task("Add configuration for Cognito (region, pool ID)", "No hardcoded AWS values"),
        ]
    )
    epic0.stories.append(story_0_2)

    # Story 0.3
    story_0_3 = Story(
        name="Story 0.3: Project Structure Refactor",
        description="Reorganize project structure for better maintainability",
        priority=2,
        tasks=[
            Task("Create internal/ directory for private packages", "Clear public/private API boundary"),
            Task("Create pkg/ directory for shared libraries", "Reusable packages across services"),
            Task("Move chat logic to internal/chat/", "Separation of concerns"),
            Task("Create internal/config/ package", "Centralized configuration"),
            Task("Add Makefile with common commands", "make build, make test, make run, make proto"),
            Task("Add .env.example file", "Document all environment variables"),
        ]
    )
    epic0.stories.append(story_0_3)

    # Story 0.4
    story_0_4 = Story(
        name="Story 0.4: Testing Foundation",
        description="Set up testing infrastructure and write initial tests",
        priority=2,
        tasks=[
            Task("Add unit test framework setup", "go test ./... works"),
            Task("Write tests for auth validation", "90%+ coverage on auth package"),
            Task("Write tests for chat server logic", "Test message broadcast, channel join/leave"),
            Task("Add integration test setup with testcontainers", "Can spin up dependencies in tests"),
            Task("Add test coverage reporting", "Coverage visible in CI"),
            Task("Create mock generators for interfaces", "Easy mocking for unit tests"),
        ]
    )
    epic0.stories.append(story_0_4)

    # Story 0.5
    story_0_5 = Story(
        name="Story 0.5: CI/CD Pipeline Setup",
        description="Set up continuous integration and deployment pipeline",
        priority=2,
        tasks=[
            Task("Create GitHub Actions workflow for tests", "Tests run on every PR"),
            Task("Add linting (golangci-lint)", "Code style enforced"),
            Task("Add security scanning (gosec)", "Security issues caught early"),
            Task("Add proto generation verification", "CI fails if generated code is stale"),
            Task("Add build workflow", "Binary builds on main branch"),
        ]
    )
    epic0.stories.append(story_0_5)

    epics.append(epic0)

    # -------------------------------------------------------------------------
    # Epic 1: Core Chat Service Hardening
    # -------------------------------------------------------------------------
    epic1 = Epic(
        name="Epic 1: Core Chat Service Hardening",
        description="Make the chat service production-ready with proper architecture",
        priority=1,
        tags=["core", "architecture"]
    )

    # Story 1.1
    story_1_1 = Story(
        name="Story 1.1: Interface-Based Architecture",
        description="Refactor to use interfaces for dependency injection and testability",
        priority=1,
        tasks=[
            Task("Define ChannelRepository interface", "Abstract channel storage"),
            Task("Define MessageRepository interface", "Abstract message storage"),
            Task("Define UserSessionManager interface", "Abstract session management"),
            Task("Define MessageBroadcaster interface", "Abstract message distribution"),
            Task("Create in-memory implementations", "Current behavior preserved"),
            Task("Use dependency injection in server", "Easy to swap implementations"),
        ]
    )
    epic1.stories.append(story_1_1)

    # Story 1.2
    story_1_2 = Story(
        name="Story 1.2: Proper gRPC Middleware",
        description="Implement proper middleware chain for cross-cutting concerns",
        priority=1,
        tasks=[
            Task("Implement auth interceptor properly", "Auth runs before all RPCs"),
            Task("Add request ID middleware", "Every request has traceable ID"),
            Task("Add logging middleware", "All RPCs logged with timing"),
            Task("Add panic recovery middleware", "Server doesn't crash on panics"),
            Task("Add rate limiting middleware (per-user)", "Configurable rate limits"),
            Task("Wire middleware chain correctly", "Correct execution order"),
        ]
    )
    epic1.stories.append(story_1_2)

    # Story 1.3
    story_1_3 = Story(
        name="Story 1.3: Enhanced Channel Management",
        description="Add full channel lifecycle management",
        priority=2,
        tasks=[
            Task("Add CreateChannel RPC", "Channels can be created via API"),
            Task("Add DeleteChannel RPC", "Channels can be deleted"),
            Task("Add ListChannels RPC", "Can list available channels"),
            Task("Add GetChannel RPC", "Can get channel details"),
            Task("Add LeaveChannel RPC", "Users can leave channels"),
            Task("Add channel types (global, zone, guild, party, whisper)", "Different channel behaviors"),
            Task("Add channel permissions model", "Who can read/write/admin"),
        ]
    )
    epic1.stories.append(story_1_3)

    # Story 1.4
    story_1_4 = Story(
        name="Story 1.4: Message Enhancements",
        description="Enhance message handling with IDs, types, and validation",
        priority=2,
        tasks=[
            Task("Add message IDs (UUID v7 for time-ordering)", "Every message uniquely identifiable"),
            Task("Add message timestamps (server-side)", "Consistent timing"),
            Task("Add message types (chat, emote, system, etc.)", "Different message rendering"),
            Task("Add message metadata (location, level, class)", "Game context in messages"),
            Task("Add message validation (length, content)", "Prevent abuse"),
            Task("Add profanity filter hook point", "Pluggable content filtering"),
        ]
    )
    epic1.stories.append(story_1_4)

    # Story 1.5
    story_1_5 = Story(
        name="Story 1.5: Connection Management",
        description="Implement robust connection lifecycle management",
        priority=1,
        tasks=[
            Task("Implement proper connection lifecycle", "Clean connect/disconnect handling"),
            Task("Add heartbeat/ping mechanism", "Detect dead connections"),
            Task("Add connection timeout handling", "Stale connections cleaned up"),
            Task("Add reconnection support (client-side hint)", "Clients know to reconnect"),
            Task("Add max connections per user limit", "Prevent connection abuse"),
            Task("Add backpressure handling", "Slow clients don't block others"),
        ]
    )
    epic1.stories.append(story_1_5)

    epics.append(epic1)

    # -------------------------------------------------------------------------
    # Epic 2: Persistence Layer
    # -------------------------------------------------------------------------
    epic2 = Epic(
        name="Epic 2: Persistence Layer",
        description="Add durable storage for messages and metadata",
        priority=1,
        tags=["persistence", "database"]
    )

    # Story 2.1
    story_2_1 = Story(
        name="Story 2.1: Database Infrastructure Setup",
        description="Set up database connections and tooling",
        priority=1,
        tasks=[
            Task("Add ScyllaDB driver dependency (gocql)", "Driver available"),
            Task("Add PostgreSQL driver dependency (pgx)", "Driver available"),
            Task("Create database connection manager", "Connection pooling, health checks"),
            Task("Add database migration tooling (golang-migrate)", "Schema changes are versioned"),
            Task("Create Docker Compose for local development", "Easy local setup"),
            Task("Add database health check endpoint", "/health includes DB status"),
        ]
    )
    epic2.stories.append(story_2_1)

    # Story 2.2
    story_2_2 = Story(
        name="Story 2.2: ScyllaDB Schema Design (Messages)",
        description="Design and implement message storage schema",
        priority=1,
        tasks=[
            Task("Design message table schema", "Optimized for time-range queries"),
            Task("Design partition strategy (channel + time bucket)", "Even data distribution"),
            Task("Add TTL for message expiration", "Old messages auto-deleted"),
            Task("Create materialized views if needed", "Support different query patterns"),
            Task("Write migration scripts", "Repeatable schema deployment"),
            Task("Add schema documentation", "Clear data model docs"),
        ]
    )
    epic2.stories.append(story_2_2)

    # Story 2.3
    story_2_3 = Story(
        name="Story 2.3: PostgreSQL Schema Design (Metadata)",
        description="Design and implement metadata storage schema",
        priority=2,
        tasks=[
            Task("Design channels table", "Store channel metadata"),
            Task("Design channel_members table", "Track membership"),
            Task("Design user_preferences table", "Per-user settings"),
            Task("Design blocked_users table", "User blocks"),
            Task("Add indexes for common queries", "Fast lookups"),
            Task("Write migration scripts", "Versioned schema changes"),
        ]
    )
    epic2.stories.append(story_2_3)

    # Story 2.4
    story_2_4 = Story(
        name="Story 2.4: Repository Implementations",
        description="Implement database-backed repositories",
        priority=1,
        tasks=[
            Task("Implement ScyllaMessageRepository", "Messages stored in ScyllaDB"),
            Task("Implement PostgresChannelRepository", "Channels stored in PostgreSQL"),
            Task("Add write-through caching layer", "Reduce read latency"),
            Task("Add batch write support", "Efficient bulk inserts"),
            Task("Add repository integration tests", "Verify against real databases"),
            Task("Add repository benchmarks", "Performance baselines"),
        ]
    )
    epic2.stories.append(story_2_4)

    # Story 2.5
    story_2_5 = Story(
        name="Story 2.5: Message History API",
        description="Implement message history retrieval",
        priority=2,
        tasks=[
            Task("Add GetMessageHistory RPC", "Fetch past messages"),
            Task("Implement cursor-based pagination", "Efficient scrolling"),
            Task("Add time-range filtering", "Fetch messages between dates"),
            Task("Add message search (optional)", "Full-text search if needed"),
            Task("Add rate limiting for history requests", "Prevent abuse"),
            Task("Cache recent messages in Redis", "Fast recent history access"),
        ]
    )
    epic2.stories.append(story_2_5)

    epics.append(epic2)

    # -------------------------------------------------------------------------
    # Epic 3: Distributed Messaging
    # -------------------------------------------------------------------------
    epic3 = Epic(
        name="Epic 3: Distributed Messaging",
        description="Enable horizontal scaling with cross-instance communication",
        priority=1,
        tags=["scaling", "distributed"]
    )

    # Story 3.1
    story_3_1 = Story(
        name="Story 3.1: Redis Infrastructure",
        description="Set up Redis for distributed state and pub/sub",
        priority=1,
        tasks=[
            Task("Add Redis driver dependency (go-redis)", "Driver available"),
            Task("Create Redis connection manager", "Connection pooling, cluster support"),
            Task("Add Redis to Docker Compose", "Local development setup"),
            Task("Add Redis Cluster configuration", "Production-ready config"),
            Task("Add Redis health check", "/health includes Redis status"),
            Task("Add Redis connection retry logic", "Handle transient failures"),
        ]
    )
    epic3.stories.append(story_3_1)

    # Story 3.2
    story_3_2 = Story(
        name="Story 3.2: Pub/Sub Message Broadcaster",
        description="Implement Redis-based message broadcasting",
        priority=1,
        tasks=[
            Task("Implement RedisMessageBroadcaster", "Messages distributed via Redis"),
            Task("Design channel naming convention", "chat:{channel_id} pattern"),
            Task("Handle subscription management", "Subscribe/unsubscribe to channels"),
            Task("Add message serialization (protobuf)", "Efficient wire format"),
            Task("Add pub/sub error handling", "Recover from Redis failures"),
            Task("Add pub/sub metrics", "Track message throughput"),
        ]
    )
    epic3.stories.append(story_3_2)

    # Story 3.3
    story_3_3 = Story(
        name="Story 3.3: Session Management",
        description="Implement distributed session storage",
        priority=1,
        tasks=[
            Task("Design session data structure", "What to store per session"),
            Task("Implement RedisSessionManager", "Sessions stored in Redis"),
            Task("Add session expiration (TTL)", "Stale sessions auto-cleaned"),
            Task("Add session refresh on activity", "Active sessions stay alive"),
            Task("Track user's current server instance", "Know where user is connected"),
            Task("Handle session migration on reconnect", "User can reconnect to different server"),
        ]
    )
    epic3.stories.append(story_3_3)

    # Story 3.4
    story_3_4 = Story(
        name="Story 3.4: Distributed Channel State",
        description="Manage channel state across instances",
        priority=2,
        tasks=[
            Task("Store channel membership in Redis", "Fast membership lookups"),
            Task("Use Redis Sets for channel members", "Efficient add/remove/list"),
            Task("Sync channel state across instances", "Consistent view of channels"),
            Task("Handle split-brain scenarios", "Graceful conflict resolution"),
            Task("Add channel member count tracking", "Know how many users per channel"),
        ]
    )
    epic3.stories.append(story_3_4)

    # Story 3.5
    story_3_5 = Story(
        name="Story 3.5: Load Balancing Support",
        description="Enable load balancing across multiple instances",
        priority=2,
        tasks=[
            Task("Make server stateless (use Redis for all state)", "Any instance can serve any user"),
            Task("Add instance registration/discovery", "Instances register themselves"),
            Task("Add sticky sessions support (optional)", "Load balancer can use if needed"),
            Task("Test with multiple instances", "Verify distributed behavior"),
            Task("Document scaling procedures", "How to add/remove instances"),
        ]
    )
    epic3.stories.append(story_3_5)

    epics.append(epic3)

    # -------------------------------------------------------------------------
    # Epic 4: Presence System
    # -------------------------------------------------------------------------
    epic4 = Epic(
        name="Epic 4: Presence System",
        description="Track player online status and real-time presence",
        priority=2,
        tags=["presence", "real-time"]
    )

    # Story 4.1
    story_4_1 = Story(
        name="Story 4.1: Presence Data Model",
        description="Design presence data structures",
        priority=2,
        tasks=[
            Task("Define presence states (online, away, busy, offline)", "Clear status options"),
            Task("Design presence data structure", "What to track per user"),
            Task("Add game-specific presence data (zone, activity)", "Rich presence info"),
            Task("Define presence update events", "When presence changes"),
            Task("Add presence proto definitions", "gRPC contracts"),
        ]
    )
    epic4.stories.append(story_4_1)

    # Story 4.2
    story_4_2 = Story(
        name="Story 4.2: Presence Storage",
        description="Implement presence storage in Redis",
        priority=2,
        tasks=[
            Task("Store presence in Redis with TTL", "Auto-offline on disconnect"),
            Task("Use Redis Hashes for presence data", "Efficient partial updates"),
            Task("Add presence heartbeat mechanism", "Detect stale presence"),
            Task("Add batch presence lookup", "Get many users' presence at once"),
            Task("Add presence indexing by guild/zone", "Find online guild members"),
        ]
    )
    epic4.stories.append(story_4_2)

    # Story 4.3
    story_4_3 = Story(
        name="Story 4.3: Presence RPCs",
        description="Implement presence gRPC endpoints",
        priority=2,
        tasks=[
            Task("Add UpdatePresence RPC", "Users can update their status"),
            Task("Add GetPresence RPC", "Get single user's presence"),
            Task("Add GetBulkPresence RPC", "Get multiple users' presence"),
            Task("Add SubscribePresence RPC (streaming)", "Real-time presence updates"),
            Task("Add GetGuildOnlineMembers RPC", "List online guild members"),
        ]
    )
    epic4.stories.append(story_4_3)

    # Story 4.4
    story_4_4 = Story(
        name="Story 4.4: Presence Notifications",
        description="Implement presence change notifications",
        priority=3,
        tasks=[
            Task("Publish presence changes to Redis", "Distributed presence events"),
            Task("Send presence updates to friends/guild", "Users notified of changes"),
            Task("Debounce rapid presence changes", "Don't spam updates"),
            Task("Add presence change rate limiting", "Prevent abuse"),
            Task("Add 'friend online' notifications", "Alert when friends log in"),
        ]
    )
    epic4.stories.append(story_4_4)

    epics.append(epic4)

    # -------------------------------------------------------------------------
    # Epic 5: Advanced Chat Features
    # -------------------------------------------------------------------------
    epic5 = Epic(
        name="Epic 5: Advanced Chat Features",
        description="MMORPG-specific chat functionality",
        priority=2,
        tags=["features", "mmorpg"]
    )

    # Story 5.1
    story_5_1 = Story(
        name="Story 5.1: Whisper (Private Messages)",
        description="Implement private messaging between players",
        priority=2,
        tasks=[
            Task("Add SendWhisper RPC", "Send private message to user"),
            Task("Create whisper channel naming (sorted user IDs)", "Consistent channel for user pair"),
            Task("Route whisper to correct server instance", "Find where recipient is connected"),
            Task("Store whisper history", "Whispers are persisted"),
            Task("Add whisper notifications (user offline)", "Queue for offline users"),
            Task("Add reply functionality (/r command hint)", "Easy reply support"),
        ]
    )
    epic5.stories.append(story_5_1)

    # Story 5.2
    story_5_2 = Story(
        name="Story 5.2: Guild Chat Integration",
        description="Integrate chat with guild system",
        priority=2,
        tasks=[
            Task("Auto-subscribe users to guild channel on login", "Guild chat available automatically"),
            Task("Validate guild membership on channel access", "Only members can read/write"),
            Task("Add guild officer channel", "Separate channel for officers"),
            Task("Handle guild membership changes", "Update subscriptions on join/leave"),
            Task("Add guild MOTD (message of the day)", "Show on login"),
        ]
    )
    epic5.stories.append(story_5_2)

    # Story 5.3
    story_5_3 = Story(
        name="Story 5.3: Party/Raid Chat",
        description="Implement party and raid group chat",
        priority=2,
        tasks=[
            Task("Add CreatePartyChannel RPC", "Create ephemeral party channel"),
            Task("Auto-cleanup when party disbands", "No orphaned channels"),
            Task("Handle party member changes", "Update subscriptions dynamically"),
            Task("Add raid channel support (larger groups)", "Scale to 40+ players"),
            Task("Add raid leader announcements", "Special broadcast ability"),
        ]
    )
    epic5.stories.append(story_5_3)

    # Story 5.4
    story_5_4 = Story(
        name="Story 5.4: Zone/Area Chat",
        description="Implement location-based chat channels",
        priority=3,
        tasks=[
            Task("Integrate with game server for player location", "Know which zone player is in"),
            Task("Auto-subscribe to zone channel on enter", "Zone chat works automatically"),
            Task("Auto-unsubscribe on zone exit", "Clean channel transitions"),
            Task("Handle high-population zones", "Scale for crowded areas"),
            Task("Add local/say chat (proximity-based)", "Only nearby players see"),
        ]
    )
    epic5.stories.append(story_5_4)

    # Story 5.5
    story_5_5 = Story(
        name="Story 5.5: System Channels",
        description="Implement server-wide system channels",
        priority=3,
        tasks=[
            Task("Add global announcement channel", "Server-wide broadcasts"),
            Task("Add trade channel", "Buying/selling"),
            Task("Add LFG (looking for group) channel", "Group finding"),
            Task("Add new player/help channel", "Newbie assistance"),
            Task("Make system channels read-only for users", "Only admins can post"),
        ]
    )
    epic5.stories.append(story_5_5)

    # Story 5.6
    story_5_6 = Story(
        name="Story 5.6: Moderation Tools",
        description="Implement chat moderation functionality",
        priority=2,
        tasks=[
            Task("Add user mute functionality", "Prevent user from sending messages"),
            Task("Add user ban functionality", "Prevent user from connecting"),
            Task("Add message deletion", "Remove inappropriate messages"),
            Task("Add chat logging for moderation", "Audit trail for reports"),
            Task("Add report message functionality", "Users can report abuse"),
            Task("Add spam detection (rate limiting + patterns)", "Auto-detect spam"),
            Task("Add profanity filter integration point", "Pluggable filter system"),
        ]
    )
    epic5.stories.append(story_5_6)

    # Story 5.7
    story_5_7 = Story(
        name="Story 5.7: User Preferences",
        description="Implement user chat preferences",
        priority=3,
        tasks=[
            Task("Add block user functionality", "Ignore specific users"),
            Task("Add channel mute (hide but stay subscribed)", "Quiet noisy channels"),
            Task("Add notification preferences", "Control what triggers alerts"),
            Task("Store preferences in PostgreSQL", "Persist across sessions"),
            Task("Sync preferences to client on connect", "Client has latest settings"),
        ]
    )
    epic5.stories.append(story_5_7)

    epics.append(epic5)

    # -------------------------------------------------------------------------
    # Epic 6: API Gateway
    # -------------------------------------------------------------------------
    epic6 = Epic(
        name="Epic 6: API Gateway",
        description="Unified entry point with security and routing",
        priority=2,
        tags=["infrastructure", "gateway"]
    )

    # Story 6.1
    story_6_1 = Story(
        name="Story 6.1: Gateway Selection & Setup",
        description="Select and configure API gateway",
        priority=2,
        tasks=[
            Task("Evaluate Envoy vs Kong vs custom Go", "Decision documented"),
            Task("Set up chosen gateway locally", "Gateway running in dev"),
            Task("Configure gRPC proxying", "gRPC traffic routed correctly"),
            Task("Configure HTTP/JSON transcoding (Connect)", "REST clients supported"),
            Task("Add TLS termination", "Encrypted external traffic"),
            Task("Add gateway to Docker Compose", "Easy local testing"),
        ]
    )
    epic6.stories.append(story_6_1)

    # Story 6.2
    story_6_2 = Story(
        name="Story 6.2: Authentication at Gateway",
        description="Implement gateway-level authentication",
        priority=1,
        tasks=[
            Task("Validate JWT at gateway level", "Invalid tokens rejected early"),
            Task("Forward user identity to backend", "Backend receives user context"),
            Task("Add token refresh handling", "Expired tokens trigger refresh"),
            Task("Add API key support (for service-to-service)", "Internal services authenticated"),
            Task("Cache Cognito JWKS", "Reduce Cognito calls"),
        ]
    )
    epic6.stories.append(story_6_2)

    # Story 6.3
    story_6_3 = Story(
        name="Story 6.3: Rate Limiting",
        description="Implement rate limiting at gateway",
        priority=2,
        tasks=[
            Task("Add global rate limiting", "Protect from DDoS"),
            Task("Add per-user rate limiting", "Prevent individual abuse"),
            Task("Add per-endpoint rate limiting", "Different limits per RPC"),
            Task("Configure rate limit storage (Redis)", "Distributed rate limiting"),
            Task("Add rate limit headers in response", "Clients know their limits"),
            Task("Add rate limit bypass for internal services", "Service mesh trusted"),
        ]
    )
    epic6.stories.append(story_6_3)

    # Story 6.4
    story_6_4 = Story(
        name="Story 6.4: Load Balancing",
        description="Configure gateway load balancing",
        priority=2,
        tasks=[
            Task("Configure round-robin load balancing", "Traffic distributed evenly"),
            Task("Add health check probes", "Unhealthy instances removed"),
            Task("Configure connection pooling", "Efficient backend connections"),
            Task("Add circuit breaker", "Protect from cascading failures"),
            Task("Configure retry policies", "Transient failures retried"),
        ]
    )
    epic6.stories.append(story_6_4)

    # Story 6.5
    story_6_5 = Story(
        name="Story 6.5: Observability at Gateway",
        description="Add monitoring and logging at gateway",
        priority=2,
        tasks=[
            Task("Add request logging", "All requests logged"),
            Task("Add metrics (Prometheus format)", "Gateway metrics available"),
            Task("Add distributed tracing headers", "Trace ID propagated"),
            Task("Add access logs", "Security audit trail"),
            Task("Create gateway dashboard (Grafana)", "Visual monitoring"),
        ]
    )
    epic6.stories.append(story_6_5)

    epics.append(epic6)

    # -------------------------------------------------------------------------
    # Epic 7: Discord Integration
    # -------------------------------------------------------------------------
    epic7 = Epic(
        name="Epic 7: Discord Integration",
        description="Allow linking game guilds/parties to Discord",
        priority=3,
        tags=["integration", "discord"]
    )

    # Story 7.1
    story_7_1 = Story(
        name="Story 7.1: Discord Bot Setup",
        description="Create and configure Discord bot",
        priority=3,
        tasks=[
            Task("Create Discord application and bot", "Bot credentials available"),
            Task("Choose Discord library (discordgo)", "Library integrated"),
            Task("Implement bot connection and event handling", "Bot comes online"),
            Task("Add bot commands framework", "Slash commands work"),
            Task("Create separate Discord service (microservice)", "Isolated from chat service"),
        ]
    )
    epic7.stories.append(story_7_1)

    # Story 7.2
    story_7_2 = Story(
        name="Story 7.2: Guild-Discord Linking",
        description="Implement guild to Discord server linking",
        priority=3,
        tasks=[
            Task("Create linking flow (guild leader initiates)", "Link guild to Discord server"),
            Task("Store guild-Discord mappings", "Persist in PostgreSQL"),
            Task("Generate Discord invite links", "Easy join for members"),
            Task("Add /joindiscord in-game command", "Opens Discord invite"),
            Task("Add role sync (optional)", "Game ranks to Discord roles"),
        ]
    )
    epic7.stories.append(story_7_2)

    # Story 7.3
    story_7_3 = Story(
        name="Story 7.3: Party/Raid Discord Integration",
        description="Temporary Discord channels for parties",
        priority=4,
        tasks=[
            Task("Create temporary voice channel for party", "Auto-create on party form"),
            Task("Generate one-time invite link", "Party members can join"),
            Task("Auto-delete channel when party disbands", "Clean up resources"),
            Task("Add party leader controls", "Leader manages channel"),
        ]
    )
    epic7.stories.append(story_7_3)

    epics.append(epic7)

    # -------------------------------------------------------------------------
    # Epic 8: Observability
    # -------------------------------------------------------------------------
    epic8 = Epic(
        name="Epic 8: Observability",
        description="Production-grade monitoring, logging, and tracing",
        priority=2,
        tags=["observability", "monitoring"]
    )

    # Story 8.1
    story_8_1 = Story(
        name="Story 8.1: Structured Logging",
        description="Implement production-ready logging",
        priority=1,
        tasks=[
            Task("Add structured logging library (zerolog/zap)", "JSON log output"),
            Task("Add request context to all logs", "Trace ID, user ID in logs"),
            Task("Add log levels (debug, info, warn, error)", "Configurable verbosity"),
            Task("Add sensitive data redaction", "No secrets in logs"),
            Task("Configure log output (stdout for containers)", "Container-friendly logging"),
            Task("Add log sampling for high-volume paths", "Reduce log volume"),
        ]
    )
    epic8.stories.append(story_8_1)

    # Story 8.2
    story_8_2 = Story(
        name="Story 8.2: Metrics",
        description="Implement Prometheus metrics",
        priority=2,
        tasks=[
            Task("Add Prometheus client library", "Metrics exposed"),
            Task("Add /metrics endpoint", "Prometheus can scrape"),
            Task("Add RPC metrics (latency, count, errors)", "Per-RPC visibility"),
            Task("Add business metrics (messages/sec, users online)", "Chat-specific metrics"),
            Task("Add database metrics", "Query latency, pool stats"),
            Task("Add Redis metrics", "Pub/sub throughput"),
        ]
    )
    epic8.stories.append(story_8_2)

    # Story 8.3
    story_8_3 = Story(
        name="Story 8.3: Distributed Tracing",
        description="Implement end-to-end tracing",
        priority=2,
        tasks=[
            Task("Add OpenTelemetry SDK", "Tracing framework integrated"),
            Task("Configure Jaeger exporter", "Traces sent to Jaeger"),
            Task("Add gRPC interceptor for tracing", "All RPCs traced"),
            Task("Add database call tracing", "DB queries in traces"),
            Task("Add Redis call tracing", "Cache operations traced"),
            Task("Propagate trace context across services", "End-to-end traces"),
        ]
    )
    epic8.stories.append(story_8_3)

    # Story 8.4
    story_8_4 = Story(
        name="Story 8.4: Alerting",
        description="Set up alerting for SLO breaches",
        priority=3,
        tasks=[
            Task("Define SLIs and SLOs", "Clear reliability targets"),
            Task("Create Prometheus alerting rules", "Alerts on SLO breach"),
            Task("Configure alert routing (PagerDuty/Slack)", "Team notified"),
            Task("Create runbooks for common alerts", "On-call guidance"),
            Task("Add dashboard for SLO tracking", "Visualize reliability"),
        ]
    )
    epic8.stories.append(story_8_4)

    # Story 8.5
    story_8_5 = Story(
        name="Story 8.5: Dashboards",
        description="Create monitoring dashboards",
        priority=3,
        tasks=[
            Task("Create Grafana dashboard for chat service", "Real-time visibility"),
            Task("Create dashboard for infrastructure", "Redis, ScyllaDB, PostgreSQL"),
            Task("Create dashboard for gateway", "Traffic patterns"),
            Task("Create business metrics dashboard", "User activity, peak times"),
            Task("Add dashboard provisioning (IaC)", "Dashboards version controlled"),
        ]
    )
    epic8.stories.append(story_8_5)

    epics.append(epic8)

    # -------------------------------------------------------------------------
    # Epic 9: Deployment & Infrastructure
    # -------------------------------------------------------------------------
    epic9 = Epic(
        name="Epic 9: Deployment & Infrastructure",
        description="Production-ready deployment pipeline",
        priority=2,
        tags=["devops", "infrastructure"]
    )

    # Story 9.1
    story_9_1 = Story(
        name="Story 9.1: Containerization",
        description="Create Docker images",
        priority=1,
        tasks=[
            Task("Create multi-stage Dockerfile", "Small, secure image"),
            Task("Add non-root user in container", "Security best practice"),
            Task("Add health check in Dockerfile", "Container health visible"),
            Task("Optimize image layers", "Fast builds, small size"),
            Task("Add .dockerignore", "Exclude unnecessary files"),
            Task("Set up container registry (ECR/GCR)", "Images stored securely"),
        ]
    )
    epic9.stories.append(story_9_1)

    # Story 9.2
    story_9_2 = Story(
        name="Story 9.2: Kubernetes Manifests",
        description="Create Kubernetes deployment manifests",
        priority=2,
        tasks=[
            Task("Create Deployment manifest", "Pod spec defined"),
            Task("Create Service manifest", "Internal networking"),
            Task("Create ConfigMap for configuration", "Config externalized"),
            Task("Create Secret management (external-secrets)", "Secrets from vault"),
            Task("Create HorizontalPodAutoscaler", "Auto-scaling configured"),
            Task("Create PodDisruptionBudget", "Safe rollouts"),
            Task("Add resource requests and limits", "Proper resource allocation"),
            Task("Add readiness and liveness probes", "Health checks configured"),
        ]
    )
    epic9.stories.append(story_9_2)

    # Story 9.3
    story_9_3 = Story(
        name="Story 9.3: Helm Chart",
        description="Create Helm chart for deployment",
        priority=3,
        tasks=[
            Task("Create Helm chart structure", "Chart scaffolded"),
            Task("Parameterize all configuration", "Values file controls all"),
            Task("Add chart dependencies (Redis, etc.)", "Full stack deployable"),
            Task("Add environment-specific values files", "Dev, staging, prod configs"),
            Task("Add chart tests", "Verify deployment works"),
            Task("Document chart usage", "Clear installation guide"),
        ]
    )
    epic9.stories.append(story_9_3)

    # Story 9.4
    story_9_4 = Story(
        name="Story 9.4: CI/CD Pipeline",
        description="Set up deployment pipeline",
        priority=2,
        tasks=[
            Task("Add Docker build to CI", "Images built on push"),
            Task("Add image scanning (Trivy)", "Vulnerabilities detected"),
            Task("Add image push to registry", "Images available for deploy"),
            Task("Create deployment pipeline (ArgoCD/Flux)", "GitOps deployment"),
            Task("Add staging environment auto-deploy", "Test before prod"),
            Task("Add production deployment approval", "Manual gate for prod"),
            Task("Add rollback procedures", "Quick recovery"),
        ]
    )
    epic9.stories.append(story_9_4)

    # Story 9.5
    story_9_5 = Story(
        name="Story 9.5: Infrastructure as Code",
        description="Automate infrastructure provisioning",
        priority=3,
        tasks=[
            Task("Create Terraform for cloud resources", "AWS/GCP infra automated"),
            Task("Create Terraform for Kubernetes cluster", "Cluster provisioned via IaC"),
            Task("Create Terraform for databases", "Managed ScyllaDB/PostgreSQL"),
            Task("Create Terraform for Redis", "Managed Redis/ElastiCache"),
            Task("Set up Terraform state management", "Remote state, locking"),
            Task("Add Terraform to CI/CD", "Plan on PR, apply on merge"),
        ]
    )
    epic9.stories.append(story_9_5)

    epics.append(epic9)

    # -------------------------------------------------------------------------
    # Epic 10: Unreal Engine Integration
    # -------------------------------------------------------------------------
    epic10 = Epic(
        name="Epic 10: Unreal Engine Integration",
        description="Client SDK for game integration",
        priority=2,
        tags=["client", "unreal"]
    )

    # Story 10.1
    story_10_1 = Story(
        name="Story 10.1: Protocol Buffer Generation for C++",
        description="Generate C++ code from protobufs",
        priority=2,
        tasks=[
            Task("Set up protoc with C++ plugin", "C++ code generated"),
            Task("Generate gRPC client stubs", "Client code available"),
            Task("Create build integration (CMake)", "Builds with UE project"),
            Task("Test generated code compiles with UE", "No compiler errors"),
            Task("Document generation process", "Reproducible builds"),
        ]
    )
    epic10.stories.append(story_10_1)

    # Story 10.2
    story_10_2 = Story(
        name="Story 10.2: Unreal Chat Plugin Structure",
        description="Create Unreal Engine plugin scaffolding",
        priority=2,
        tasks=[
            Task("Create UE plugin scaffolding", "Plugin loads in editor"),
            Task("Add gRPC dependencies to plugin", "gRPC library linked"),
            Task("Create ChatSubsystem (GameInstanceSubsystem)", "Lifecycle managed"),
            Task("Add async task support", "Non-blocking operations"),
            Task("Add Blueprint exposure (optional)", "Designer-friendly API"),
        ]
    )
    epic10.stories.append(story_10_2)

    # Story 10.3
    story_10_3 = Story(
        name="Story 10.3: Connection Management",
        description="Implement client connection handling",
        priority=2,
        tasks=[
            Task("Implement connection to chat server", "Can establish connection"),
            Task("Add authentication header injection", "JWT sent with requests"),
            Task("Implement reconnection logic", "Auto-reconnect on disconnect"),
            Task("Add connection state events", "UI can show connection status"),
            Task("Handle network errors gracefully", "No crashes on network issues"),
        ]
    )
    epic10.stories.append(story_10_3)

    # Story 10.4
    story_10_4 = Story(
        name="Story 10.4: Chat API Wrapper",
        description="Create UE-friendly API wrappers",
        priority=2,
        tasks=[
            Task("Implement JoinChannel wrapper", "Easy channel joining"),
            Task("Implement SendMessage wrapper", "Easy message sending"),
            Task("Implement message receive handler", "Messages delivered to UI"),
            Task("Implement GetHistory wrapper", "Load chat history"),
            Task("Add delegate/event system", "UE-friendly callbacks"),
            Task("Add message queue for offline/reconnect", "Buffer messages during disconnect"),
        ]
    )
    epic10.stories.append(story_10_4)

    # Story 10.5
    story_10_5 = Story(
        name="Story 10.5: UI Integration Support",
        description="Provide UI integration examples",
        priority=3,
        tasks=[
            Task("Create sample chat widget (UMG)", "Reference implementation"),
            Task("Add chat input handling", "Text input works"),
            Task("Add chat display (scrolling, history)", "Messages displayed"),
            Task("Add channel tabs/switching", "Multi-channel UI"),
            Task("Add user presence display", "Online indicators work"),
            Task("Document UI integration patterns", "Guide for game UI team"),
        ]
    )
    epic10.stories.append(story_10_5)

    epics.append(epic10)

    # -------------------------------------------------------------------------
    # Epic 11: Future Microservices (Post-MVP)
    # -------------------------------------------------------------------------
    epic11 = Epic(
        name="Epic 11: Future Microservices Architecture",
        description="Split into specialized services as complexity grows (post-MVP planning)",
        priority=4,
        tags=["future", "architecture"]
    )

    # Story 11.1
    story_11_1 = Story(
        name="Story 11.1: Service Boundaries Definition",
        description="Plan microservices architecture for future",
        priority=4,
        tasks=[
            Task("Document service boundaries", "Clear ownership per service"),
            Task("Define service communication contracts", "gRPC service definitions"),
            Task("Plan data ownership per service", "No shared databases"),
            Task("Design event-driven communication", "Async where appropriate"),
        ]
    )
    epic11.stories.append(story_11_1)

    epics.append(epic11)

    return epics

# ============================================================================
# Main Setup Logic
# ============================================================================

def find_or_create(items: list, name: str, create_fn) -> dict:
    """Find an item by name or create it"""
    for item in items:
        if item.get("name", "").lower() == name.lower():
            print(f"  Found existing: {name}")
            return item
    print(f"  Creating: {name}")
    return create_fn()

def test_api_connection(api_key: str):
    """Test API connection and diagnose issues"""
    print("=" * 60)
    print("ClickUp API Connection Test")
    print("=" * 60)

    client = ClickUpClient(api_key)
    results = client.test_connection()

    print(f"\nAPI Key Format: {results['api_key_format']}")
    print(f"\nTesting Endpoints:")
    print("-" * 40)

    for test in results["endpoints_tested"]:
        status_icon = "[OK]" if test["success"] else "[FAIL]"
        print(f"\n{status_icon} {test['description']}")
        print(f"    Endpoint: {BASE_URL}/{test['endpoint']}")
        print(f"    Status: {test['status']}")
        print(f"    Response: {test['response'][:100]}...")

    print("\n" + "=" * 60)

    # Provide recommendations
    any_success = any(t["success"] for t in results["endpoints_tested"])

    if any_success:
        print("API connection successful!")
        # Find which endpoint works
        for test in results["endpoints_tested"]:
            if test["success"]:
                print(f"  Working endpoint: {test['endpoint']}")
    else:
        print("API connection FAILED. Possible causes:")
        print("  1. Invalid API key - regenerate at ClickUp > Settings > Apps > API Token")
        print("  2. API key expired - generate a new one")
        print("  3. Network issue - check your internet connection")
        print("  4. ClickUp API is down - check https://status.clickup.com")
        print("\nTo find your Team ID manually:")
        print("  1. Go to ClickUp in your browser")
        print("  2. Look at the URL: https://app.clickup.com/XXXXXXX/...")
        print("  3. The number after app.clickup.com/ is your Team ID")
        print("  4. Run: python clickup_setup.py --api-key YOUR_KEY --team-id XXXXXXX")

    return any_success


def setup_clickup(api_key: str, dry_run: bool = False, team_id: str = None):
    """Main setup function"""
    global DEBUG

    print("=" * 60)
    print("ClickUp Project Setup for Hermes Chat Engine")
    print("=" * 60)

    client = ClickUpClient(api_key)

    # Step 1: Find workspace
    print("\n[1/6] Finding workspace...")

    if team_id:
        # User provided team_id directly
        print(f"  Using provided Team ID: {team_id}")
    else:
        # Try to get teams via API
        try:
            teams = client.get_teams()
            team = None
            for t in teams:
                if t["name"].lower() == WORKSPACE_NAME.lower():
                    team = t
                    break

            if not team:
                print(f"  ERROR: Workspace '{WORKSPACE_NAME}' not found!")
                print(f"  Available workspaces: {[t['name'] for t in teams]}")
                sys.exit(1)

            print(f"  Found workspace: {team['name']} (ID: {team['id']})")
            team_id = team["id"]
        except Exception as e:
            print(f"  ERROR: Could not fetch workspaces: {e}")
            print("\n  WORKAROUND: Find your Team ID manually:")
            print("    1. Go to ClickUp in your browser")
            print("    2. Look at the URL: https://app.clickup.com/XXXXXXX/...")
            print("    3. The number after app.clickup.com/ is your Team ID")
            print("    4. Run: python clickup_setup.py --api-key YOUR_KEY --team-id XXXXXXX")
            print("\n  Or run with --test to diagnose the API connection:")
            print("    python clickup_setup.py --api-key YOUR_KEY --test")
            sys.exit(1)

    # Step 2: Find or create space
    print("\n[2/6] Setting up space...")
    spaces = client.get_spaces(team_id)
    space = find_or_create(
        spaces,
        SPACE_NAME,
        lambda: client.create_space(team_id, SPACE_NAME)
    )
    space_id = space["id"]

    # Step 3: Create tags in space
    print("\n[3/6] Creating tags...")
    tag_colors = {
        "foundation": "#FF6B6B",
        "p0-blocker": "#FF0000",
        "core": "#4ECDC4",
        "architecture": "#45B7D1",
        "persistence": "#96CEB4",
        "database": "#FFEAA7",
        "scaling": "#DDA0DD",
        "distributed": "#98D8C8",
        "presence": "#F7DC6F",
        "real-time": "#BB8FCE",
        "features": "#85C1E9",
        "mmorpg": "#F8B500",
        "infrastructure": "#AED6F1",
        "gateway": "#D7BDE2",
        "integration": "#A3E4D7",
        "discord": "#7289DA",
        "observability": "#FAD7A0",
        "monitoring": "#F5B041",
        "devops": "#5DADE2",
        "client": "#58D68D",
        "unreal": "#EC7063",
        "future": "#BDC3C7",
    }

    for tag_name, color in tag_colors.items():
        try:
            client.create_tag(space_id, tag_name, color)
            print(f"  Created tag: {tag_name}")
        except Exception as e:
            if "already exists" in str(e).lower() or "409" in str(e):
                print(f"  Tag exists: {tag_name}")
            else:
                print(f"  Warning: Could not create tag {tag_name}: {e}")

    # Step 4: Find or create folder
    print("\n[4/6] Setting up folder...")
    folders = client.get_folders(space_id)
    folder = find_or_create(
        folders,
        FOLDER_NAME,
        lambda: client.create_folder(space_id, FOLDER_NAME)
    )
    folder_id = folder["id"]

    # Step 5: Find or create list
    print("\n[5/6] Setting up list...")
    lists = client.get_lists(folder_id)
    task_list = find_or_create(
        lists,
        LIST_NAME,
        lambda: client.create_list(folder_id, LIST_NAME)
    )
    list_id = task_list["id"]

    # Step 6: Create Epics, Stories, and Tasks
    print("\n[6/6] Creating Epics, Stories, and Tasks...")
    epics = get_epics()

    total_stories = sum(len(e.stories) for e in epics)
    total_tasks = sum(sum(len(s.tasks) for s in e.stories) for e in epics)

    print(f"\n  Total to create:")
    print(f"    - {len(epics)} Epics")
    print(f"    - {total_stories} Stories")
    print(f"    - {total_tasks} Tasks")

    if dry_run:
        print("\n  DRY RUN - No changes made")
        return

    print("\n  Creating items (this may take a few minutes)...")

    for epic_idx, epic in enumerate(epics, 1):
        print(f"\n  [{epic_idx}/{len(epics)}] Creating Epic: {epic.name}")

        # Create Epic as a task
        epic_task = client.create_task(
            list_id=list_id,
            name=epic.name,
            description=epic.description,
            priority=epic.priority,
            tags=epic.tags
        )
        epic_id = epic_task["id"]

        # Create Stories as subtasks of Epic
        for story_idx, story in enumerate(epic.stories, 1):
            print(f"    [{story_idx}/{len(epic.stories)}] Story: {story.name}")

            story_task = client.create_task(
                list_id=list_id,
                name=story.name,
                description=story.description,
                priority=story.priority,
                parent=epic_id
            )
            story_id = story_task["id"]

            # Create Tasks as checklist items on the Story
            if story.tasks:
                checklist = client.create_checklist(story_id, "Tasks")
                checklist_id = checklist["checklist"]["id"]

                for task in story.tasks:
                    task_name = task.name
                    if task.description:
                        task_name = f"{task.name} - {task.description}"
                    client.create_checklist_item(checklist_id, task_name)

            # Small delay to avoid rate limiting
            time.sleep(0.2)

    print("\n" + "=" * 60)
    print("Setup Complete!")
    print("=" * 60)
    print(f"\nCreated:")
    print(f"  - Space: {SPACE_NAME}")
    print(f"  - Folder: {FOLDER_NAME}")
    print(f"  - List: {LIST_NAME}")
    print(f"  - {len(epics)} Epics with {total_stories} Stories and {total_tasks} Tasks")
    print(f"\nView your backlog at: https://app.clickup.com")

# ============================================================================
# CLI Entry Point
# ============================================================================

def main():
    global DEBUG

    parser = argparse.ArgumentParser(
        description="Set up ClickUp project for Hermes Chat Engine",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Test API connection first
  python clickup_setup.py --api-key pk_12345_XXXXX --test

  # Normal setup (auto-detects workspace)
  python clickup_setup.py --api-key pk_12345_XXXXX

  # Setup with manual Team ID (if auto-detect fails)
  python clickup_setup.py --api-key pk_12345_XXXXX --team-id 12345678

  # Dry run (preview without creating anything)
  python clickup_setup.py --api-key pk_12345_XXXXX --dry-run

  # Debug mode (verbose output)
  python clickup_setup.py --api-key pk_12345_XXXXX --debug

How to find your Team ID:
  1. Go to ClickUp in your browser
  2. Look at the URL: https://app.clickup.com/XXXXXXX/...
  3. The number after app.clickup.com/ is your Team ID
        """
    )

    parser.add_argument(
        "--api-key",
        required=True,
        help="ClickUp API key (get from Settings > Apps > API Token)"
    )

    parser.add_argument(
        "--test",
        action="store_true",
        help="Test API connection and diagnose issues"
    )

    parser.add_argument(
        "--team-id",
        help="Team/Workspace ID (find in ClickUp URL: app.clickup.com/XXXXX/...)"
    )

    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be created without making changes"
    )

    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable debug mode with verbose output"
    )

    args = parser.parse_args()

    if args.debug:
        DEBUG = True

    try:
        if args.test:
            success = test_api_connection(args.api_key)
            sys.exit(0 if success else 1)
        else:
            setup_clickup(args.api_key, args.dry_run, args.team_id)
    except KeyboardInterrupt:
        print("\n\nSetup cancelled by user")
        sys.exit(1)
    except Exception as e:
        print(f"\nError: {e}")
        if DEBUG:
            import traceback
            traceback.print_exc()
        sys.exit(1)

if __name__ == "__main__":
    main()
