# Release Process

This document describes the automated release process for dogfetch.

## Overview

Releases are fully automated using GitHub Actions and GoReleaser. When you push a tag, the system automatically builds, tests, and releases binaries for multiple platforms.

## Creating a Release

### 1. Ensure your changes are committed

```bash
git add .
git commit -m "Prepare for release v1.0.0"
git push origin main
```

### 2. Create and push a tag

```bash
# Create an annotated tag
git tag -a v1.0.0 -m "Release v1.0.0"

# Push the tag to trigger the release
git push origin v1.0.0
```

### 3. Wait for GitHub Actions

The release workflow will automatically:
- ✅ Run all tests
- ✅ Build binaries for:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)
  - Windows (amd64, arm64)
- ✅ Create a GitHub Release
- ✅ Upload binaries with checksums
- ✅ Generate release notes from commits

## What Gets Built

Each release includes:

- `dogfetch_1.0.0_Linux_x86_64.tar.gz`
- `dogfetch_1.0.0_Linux_arm64.tar.gz`
- `dogfetch_1.0.0_Darwin_x86_64.tar.gz`
- `dogfetch_1.0.0_Darwin_arm64.tar.gz`
- `dogfetch_1.0.0_Windows_x86_64.zip`
- `dogfetch_1.0.0_Windows_arm64.zip`
- `checksums.txt` - SHA256 checksums for verification

## Version Numbering

Follow semantic versioning (https://semver.org/):

- `v1.0.0` - Major release (breaking changes)
- `v1.1.0` - Minor release (new features, backward compatible)
- `v1.0.1` - Patch release (bug fixes)

## Pre-releases

For beta or RC versions:

```bash
git tag -a v1.0.0-beta.1 -m "Beta release"
git push origin v1.0.0-beta.1
```

GoReleaser will mark these as pre-releases on GitHub.

## Checking Release Status

After pushing a tag:

1. Go to: https://github.com/jtzemp/dogfetch/actions
2. Click on the "Release" workflow
3. Watch the progress
4. Once complete, check: https://github.com/jtzemp/dogfetch/releases

## Troubleshooting

### Release workflow didn't trigger

- Ensure the tag starts with `v` (e.g., `v1.0.0`, not `1.0.0`)
- Check that you pushed the tag: `git push origin v1.0.0`
- Verify the workflow file exists: `.github/workflows/release.yml`

### Tests failed

The release will not complete if tests fail. Fix the issues:

```bash
# Run tests locally
make test

# Fix issues, commit, and delete/recreate the tag
git tag -d v1.0.0
git push origin :refs/tags/v1.0.0
# Fix and recreate tag
```

### Build failed

Check the GitHub Actions logs for specific errors. Common issues:
- Missing dependencies
- Invalid `.goreleaser.yml` syntax
- Permission issues

## Manual Release (Testing)

To test the release process locally without creating a GitHub release:

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser@latest

# Create a snapshot release (local only)
goreleaser release --snapshot --clean

# Binaries will be in ./dist/
ls -la dist/
```

## Deleting a Release

If you need to delete a release:

1. Delete the GitHub release (via web UI)
2. Delete the tag locally: `git tag -d v1.0.0`
3. Delete the tag remotely: `git push origin :refs/tags/v1.0.0`
