# Releasing mcpinspect

This project uses [GoReleaser](https://goreleaser.com/) for automated releases via GitHub Actions.

## How It Works

The workflow in `.github/workflows/release-binary.yml` runs GoReleaser whenever you push a git tag. It builds binaries and creates a GitHub Release.

## To Release a New Version

1. **Commit your changes** and push to main:
   ```bash
   git add . && git commit -m "Your commit message"
   git push origin main
   ```

2. **Create and push a tag**:
   ```bash
   git tag v0.0.1
   git push origin v0.0.1
   ```

This triggers the workflow which:
- Checks out your code
- Runs `goreleaser release --clean`
- Creates a GitHub Release with your binaries

## Local Testing (without publishing)

To test locally without creating a release:
```bash
# Install goreleaser if needed
brew install goreleaser

# Test the build (no publish)
goreleaser release --snapshot --clean
```

The `--snapshot` flag builds without requiring a tag and doesn't publish.

## Current Build Targets

The `.goreleaser.yml` is configured to build for:
- macOS (darwin) on amd64
- macOS (darwin) on arm64

To add Linux/Windows builds, uncomment the relevant lines in the `goos` section of `.goreleaser.yml`.

## Version Numbering

GoReleaser uses the git tag as the version. Common patterns:
- `v0.0.1` - patch/initial release
- `v0.1.0` - minor release
- `v1.0.0` - major release

The `v` prefix is conventional but optional.
