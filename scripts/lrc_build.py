#!/usr/bin/env python3
"""
lrc Build and Release Automation

This script handles building the lrc CLI tool for multiple platforms,
version management based on Git commit IDs, and uploading releases to
Backblaze B2 storage using native REST APIs.
"""

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Tuple

try:
    import requests
except ImportError:
    print("Error: requests library not found. Install with: pip install requests")
    sys.exit(1)


# Build configuration
LRC_SOURCE_PATH = "."
BUILD_OUTPUT_DIR = "dist"
PLATFORMS = [
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
]

# B2 configuration
B2_API_BASE = "https://api.backblazeb2.com"
B2_UPLOAD_PATH_PREFIX = "lrc"  # Files go to hexmos/lrc/<version>/

# These are loaded from .env file (see .env.example)
B2_ENV_VARS = ["B2_KEY_ID", "B2_APP_KEY", "B2_BUCKET_NAME", "B2_BUCKET_ID"]


class LRCBuilder:
    """Handles building and releasing lrc CLI tool"""

    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        self.project_root = Path(__file__).parent.parent.resolve()
        self.lrc_path = self.project_root / LRC_SOURCE_PATH
        self.dist_dir = self.project_root / BUILD_OUTPUT_DIR
        self._load_dotenv()

    def _load_dotenv(self):
        """Load .env file from project root into os.environ (existing vars take precedence)"""
        env_file = self.project_root / ".env"
        if not env_file.exists():
            return
        with open(env_file, "r") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" not in line:
                    continue
                key, _, value = line.partition("=")
                key = key.strip()
                value = value.strip()
                # Don't override existing environment variables
                if key not in os.environ:
                    os.environ[key] = value

    def _validate_b2_config(self):
        """Validate that all required B2 env vars are set"""
        missing = [v for v in B2_ENV_VARS if not os.environ.get(v)]
        if missing:
            print(f"Error: Missing B2 configuration: {', '.join(missing)}")
            print(f"Create a .env file in the project root (see .env.example)")
            sys.exit(1)

    def log(self, message: str, force: bool = False):
        """Print message if verbose or force"""
        if self.verbose or force:
            print(message)

    def run_command(
        self, cmd: List[str], cwd: Optional[Path] = None, check: bool = True
    ) -> Tuple[int, str, str]:
        """Run a shell command and return exit code, stdout, stderr"""
        if self.verbose:
            self.log(f"Running: {' '.join(cmd)}")
        
        result = subprocess.run(
            cmd,
            cwd=cwd or self.project_root,
            capture_output=True,
            text=True,
        )
        
        if check and result.returncode != 0:
            print(f"Error running command: {' '.join(cmd)}")
            print(f"Exit code: {result.returncode}")
            print(f"Stderr: {result.stderr}")
            sys.exit(1)
        
        return result.returncode, result.stdout.strip(), result.stderr.strip()

    def check_lrc_clean(self) -> bool:
        """Check if repo has uncommitted changes"""
        self.log("Checking for uncommitted changes...")
        
        # Check if there are any unstaged changes
        code, stdout, _ = self.run_command(
            ["git", "diff", "--quiet"],
            check=False
        )
        if code != 0:
            return False
        
        # Check if there are any staged but uncommitted changes
        code, stdout, _ = self.run_command(
            ["git", "diff", "--cached", "--quiet"],
            check=False
        )
        if code != 0:
            return False
        
        # Check for untracked files
        _, stdout, _ = self.run_command(
            ["git", "ls-files", "--others", "--exclude-standard"]
        )
        if stdout:
            return False
        
        return True

    def get_commit_id(self) -> str:
        """Get current Git commit ID (short SHA)"""
        _, stdout, _ = self.run_command(["git", "rev-parse", "--short=7", "HEAD"])
        return stdout

    def get_version_from_source(self) -> str:
        """Extract version from main.go"""
        main_go = self.lrc_path / "main.go"
        if not main_go.exists():
            print(f"Error: {main_go} not found")
            sys.exit(1)
        
        with open(main_go, "r") as f:
            for line in f:
                if line.strip().startswith("const appVersion"):
                    # Extract version from: const appVersion = "v1.0.0"
                    parts = line.split('"')
                    if len(parts) >= 2:
                        return parts[1]
        
        print("Error: Could not find appVersion in main.go")
        sys.exit(1)

    def parse_version(self, version: str) -> Tuple[int, int, int]:
        """Parse semantic version string (v1.2.3) into tuple (1, 2, 3)"""
        if not version.startswith("v"):
            print(f"Error: Version must start with 'v': {version}")
            sys.exit(1)
        
        parts = version[1:].split(".")
        if len(parts) != 3:
            print(f"Error: Version must be in format vX.Y.Z: {version}")
            sys.exit(1)
        
        try:
            return (int(parts[0]), int(parts[1]), int(parts[2]))
        except ValueError:
            print(f"Error: Invalid version format: {version}")
            sys.exit(1)

    def bump_version(self, current: str, bump_type: str) -> str:
        """Bump version based on type (patch, minor, major)"""
        major, minor, patch = self.parse_version(current)
        
        if bump_type == "patch":
            patch += 1
        elif bump_type == "minor":
            minor += 1
            patch = 0
        elif bump_type == "major":
            major += 1
            minor = 0
            patch = 0
        else:
            print(f"Error: Invalid bump type: {bump_type}")
            sys.exit(1)
        
        return f"v{major}.{minor}.{patch}"

    def update_version_in_source(self, new_version: str):
        """Update appVersion constant in main.go"""
        main_go = self.lrc_path / "main.go"
        
        with open(main_go, "r") as f:
            lines = f.readlines()
        
        updated = False
        for i, line in enumerate(lines):
            if line.strip().startswith("const appVersion"):
                # Replace the version while preserving the comment
                if "//" in line:
                    lines[i] = f'const appVersion = "{new_version}" // Semantic version - bump this for releases\n'
                else:
                    lines[i] = f'const appVersion = "{new_version}"\n'
                updated = True
                break
        
        if not updated:
            print("Error: Could not update appVersion in main.go")
            sys.exit(1)
        
        with open(main_go, "w") as f:
            f.writelines(lines)
        
        self.log(f"Updated {main_go} to version {new_version}", force=True)

    def get_build_time(self) -> str:
        """Get current build timestamp"""
        return datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")

    def build_for_platform(
        self, goos: str, goarch: str, version: str, build_time: str, commit: str
    ) -> Tuple[Path, str]:
        """Build lrc for a specific platform
        
        Returns: (binary_path, platform_dir) tuple
        """
        # Create platform directory: linux-amd64, darwin-arm64, etc.
        platform_dir = f"{goos}-{goarch}"
        platform_path = self.dist_dir / platform_dir
        platform_path.mkdir(parents=True, exist_ok=True)
        
        # Simple binary name: lrc or lrc.exe
        binary_name = "lrc.exe" if goos == "windows" else "lrc"
        output_path = platform_path / binary_name
        
        self.log(f"Building {goos}/{goarch}...", force=True)
        
        # Prepare ldflags for version injection
        ldflags = (
            f"-X main.version={version} "
            f"-X main.buildTime={build_time} "
            f"-X main.gitCommit={commit}"
        )
        
        # Build command
        env = os.environ.copy()
        env["CGO_ENABLED"] = "0"
        env["GOOS"] = goos
        env["GOARCH"] = goarch
        
        cmd = [
            "go", "build",
            "-ldflags", ldflags,
            "-o", str(output_path),
            "."
        ]
        
        result = subprocess.run(
            cmd,
            cwd=self.project_root,
            env=env,
            capture_output=True,
            text=True,
        )
        
        if result.returncode != 0:
            print(f"Error building for {goos}/{goarch}:")
            print(result.stderr)
            sys.exit(1)
        
        # Compute SHA256 checksum
        sha256_hash = hashlib.sha256()
        with open(output_path, "rb") as f:
            for chunk in iter(lambda: f.read(4096), b""):
                sha256_hash.update(chunk)
        
        checksum = sha256_hash.hexdigest()
        self.log(f"  ✓ Built {platform_dir}/{binary_name} (SHA256: {checksum[:16]}...)")
        
        return output_path, platform_dir

    def build_cross_platform(self, version: str) -> List[Tuple[Path, str]]:
        """Build for all platforms
        
        Returns: List of (binary_path, platform_dir) tuples
        """
        self.log("Starting cross-platform build...", force=True)
        
        # Clean and create dist directory
        if self.dist_dir.exists():
            shutil.rmtree(self.dist_dir)
        self.dist_dir.mkdir(parents=True)
        
        build_time = self.get_build_time()
        commit = self.get_commit_id()
        
        built_files = []
        for goos, goarch in PLATFORMS:
            binary_path, platform_dir = self.build_for_platform(goos, goarch, version, build_time, commit)
            built_files.append((binary_path, platform_dir))
        
        # Generate SHA256SUMS file in each platform directory
        for binary_path, platform_dir in built_files:
            sums_file = binary_path.parent / "SHA256SUMS"
            sha256_hash = hashlib.sha256()
            with open(binary_path, "rb") as bf:
                for chunk in iter(lambda: bf.read(4096), b""):
                    sha256_hash.update(chunk)
            checksum = sha256_hash.hexdigest()
            with open(sums_file, "w") as f:
                f.write(f"{checksum}  {binary_path.name}\n")
        
        self.log(f"\n✓ Build complete! {len(built_files)} platform builds in {self.dist_dir}", force=True)
        
        return built_files

    def authorize_b2(self) -> Dict:
        """Authorize with B2 and get auth token"""
        self.log("Authorizing with Backblaze B2...")
        
        url = f"{B2_API_BASE}/b2api/v2/b2_authorize_account"
        
        response = requests.get(
            url,
            auth=(os.environ["B2_KEY_ID"], os.environ["B2_APP_KEY"]),
            timeout=30
        )
        
        if response.status_code != 200:
            print(f"Error authorizing with B2: {response.status_code}")
            print(response.text)
            sys.exit(1)
        
        data = response.json()
        self.log(f"✓ Authorized with B2 (Account: {data.get('accountId', 'unknown')})")
        return data

    def find_bucket_id(self, auth_data: Dict, bucket_name: str) -> str:
        """Find bucket ID by name"""
        self.log(f"Looking up bucket '{bucket_name}'...")
        
        url = f"{auth_data['apiUrl']}/b2api/v2/b2_list_buckets"
        
        response = requests.post(
            url,
            headers={"Authorization": auth_data["authorizationToken"]},
            json={"accountId": auth_data["accountId"]},
            timeout=30
        )
        
        if response.status_code != 200:
            print(f"Error listing buckets: {response.status_code}")
            print(response.text)
            sys.exit(1)
        
        buckets = response.json().get("buckets", [])
        for bucket in buckets:
            if bucket["bucketName"] == bucket_name:
                bucket_id = bucket["bucketId"]
                self.log(f"✓ Found bucket '{bucket_name}' (ID: {bucket_id})")
                return bucket_id
        
        print(f"Error: Bucket '{bucket_name}' not found")
        print(f"Available buckets: {[b['bucketName'] for b in buckets]}")
        sys.exit(1)

    def get_upload_url(self, auth_data: Dict, bucket_id: str) -> Dict:
        """Get upload URL for B2 bucket"""
        self.log(f"Getting upload URL for bucket {bucket_id}...")
        
        url = f"{auth_data['apiUrl']}/b2api/v2/b2_get_upload_url"
        
        response = requests.post(
            url,
            headers={"Authorization": auth_data["authorizationToken"]},
            json={"bucketId": bucket_id},
            timeout=30
        )
        
        if response.status_code != 200:
            print(f"Error getting upload URL: {response.status_code}")
            print(response.text)
            sys.exit(1)
        
        return response.json()

    def upload_file_to_b2(
        self, upload_data: Dict, file_path: Path, b2_file_name: str
    ) -> Dict:
        """Upload a file to B2"""
        self.log(f"Uploading {file_path.name} as {b2_file_name}...")
        
        # Read file and compute SHA1 (required by B2)
        with open(file_path, "rb") as f:
            file_data = f.read()
        
        sha1_hash = hashlib.sha1(file_data).hexdigest()
        
        headers = {
            "Authorization": upload_data["authorizationToken"],
            "X-Bz-File-Name": b2_file_name,
            "Content-Type": "application/octet-stream",
            "X-Bz-Content-Sha1": sha1_hash,
            "X-Bz-Info-src_last_modified_millis": str(int(file_path.stat().st_mtime * 1000)),
        }
        
        response = requests.post(
            upload_data["uploadUrl"],
            headers=headers,
            data=file_data,
            timeout=300  # 5 minutes for large files
        )
        
        if response.status_code != 200:
            print(f"Error uploading file: {response.status_code}")
            print(response.text)
            sys.exit(1)
        
        self.log(f"  ✓ Uploaded {file_path.name}", force=True)
        return response.json()

    def upload_to_b2(self, files: List[Tuple[Path, str]], version: str):
        """Upload all built files to B2 using credentials from .env
        
        Args:
            files: List of (binary_path, platform_dir) tuples
            version: Semantic version string (e.g., "v1.0.0")
        """
        self._validate_b2_config()
        self.log("Starting B2 upload...", force=True)
        
        # Authorize with credentials from .env
        auth_data = self.authorize_b2()
        
        bucket_id = os.environ["B2_BUCKET_ID"]
        self.log(f"Using bucket ID: {bucket_id}")
        
        # Upload each platform's files (binary + SHA256SUMS)
        for binary_path, platform_dir in files:
            # Get fresh upload URL for binary
            upload_data = self.get_upload_url(auth_data, bucket_id)
            
            # B2 path: lrc/<version>/<platform>/lrc (or lrc.exe)
            b2_file_name = f"{B2_UPLOAD_PATH_PREFIX}/{version}/{platform_dir}/{binary_path.name}"
            
            # Upload binary
            self.upload_file_to_b2(upload_data, binary_path, b2_file_name)
            
            # Upload SHA256SUMS for this platform
            sums_file = binary_path.parent / "SHA256SUMS"
            if sums_file.exists():
                upload_data = self.get_upload_url(auth_data, bucket_id)
                b2_sums_name = f"{B2_UPLOAD_PATH_PREFIX}/{version}/{platform_dir}/SHA256SUMS"
                self.upload_file_to_b2(upload_data, sums_file, b2_sums_name)
        
        # Construct public download URLs
        download_base = f"https://f005.backblazeb2.com/file/{os.environ['B2_BUCKET_NAME']}/{B2_UPLOAD_PATH_PREFIX}/{version}"
        
        self.log(f"\n✓ Upload complete! Files available at:", force=True)
        self.log(f"  {download_base}/", force=True)
        self.log(f"\nPlatform directories:", force=True)
        for _, platform_dir in files:
            self.log(f"  {download_base}/{platform_dir}/", force=True)

    def cmd_build(self, args):
        """Build lrc binaries"""
        # Check for clean working directory
        if not self.check_lrc_clean():
            print("Error: Repository has uncommitted changes")
            print("Please commit or stash changes before building")
            sys.exit(1)
        
        # Get version from source code
        version = self.get_version_from_source()
        commit = self.get_commit_id()
        self.log(f"Building version {version} (commit: {commit})", force=True)
        
        # Build for all platforms
        built_files = self.build_cross_platform(version)
        
        return version, built_files

    def cmd_bump(self, args):
        """Bump version in main.go"""
        # Check for clean working directory
        if not self.check_lrc_clean():
            print("Error: Repository has uncommitted changes")
            print("Please commit or stash changes before bumping version")
            sys.exit(1)
        
        # Get current version
        current_version = self.get_version_from_source()
        self.log(f"Current version: {current_version}", force=True)
        
        # Prompt for bump type
        print("\nSelect version bump type:")
        print("  1. patch (bug fixes, small changes)")
        print("  2. minor (new features, backward compatible)")
        print("  3. major (breaking changes)")
        
        while True:
            choice = input("\nEnter choice [1/2/3] or [patch/minor/major]: ").strip().lower()
            
            if choice in ["1", "patch"]:
                bump_type = "patch"
                break
            elif choice in ["2", "minor"]:
                bump_type = "minor"
                break
            elif choice in ["3", "major"]:
                bump_type = "major"
                break
            else:
                print("Invalid choice. Please enter 1, 2, 3, patch, minor, or major")
        
        # Calculate new version
        new_version = self.bump_version(current_version, bump_type)
        self.log(f"New version: {new_version}", force=True)
        
        # Confirm
        confirm = input(f"\nBump {current_version} → {new_version}? [y/N]: ").strip().lower()
        if confirm != "y":
            print("Aborted")
            sys.exit(0)
        
        # Update version in source
        self.update_version_in_source(new_version)
        
        # Commit the change
        self.log("Committing version bump...", force=True)
        self.run_command(["git", "add", "main.go"])
        # Run lrc review skip to record attestation before committing
        self.run_command(["git", "lrc", "review", "--skip"])
        self.run_command([
            "git", "commit", "-m",
            f"lrc: bump version {current_version} → {new_version}"
        ])
        
        self.log(f"\n✅ Version bumped to {new_version}", force=True)
        self.log("Run 'make build-all' to build this version", force=True)

    def cmd_release(self, args):
        """Build and upload to B2 (using hardcoded credentials)"""
        # Build
        version, built_files = self.cmd_build(args)
        
        # Upload using hardcoded credentials
        self.upload_to_b2(built_files, version)
        
        self.log("\n🎉 Release complete!", force=True)


def main():
    parser = argparse.ArgumentParser(
        description="Build and release lrc CLI tool",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    
    parser.add_argument(
        "-v", "--verbose",
        action="store_true",
        help="Enable verbose output"
    )
    
    subparsers = parser.add_subparsers(dest="command", help="Command to run")
    
    # Build command
    build_parser = subparsers.add_parser("build", help="Build lrc for all platforms")
    
    # Bump command
    bump_parser = subparsers.add_parser("bump", help="Bump version in main.go")
    
    # Release command
    release_parser = subparsers.add_parser(
        "release",
        help="Build and upload to Backblaze B2"
    )
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        sys.exit(1)
    
    builder = LRCBuilder(verbose=args.verbose)
    
    if args.command == "build":
        builder.cmd_build(args)
    elif args.command == "bump":
        builder.cmd_bump(args)
    elif args.command == "release":
        builder.cmd_release(args)
    else:
        parser.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
