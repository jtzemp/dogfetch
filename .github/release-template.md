## What's Changed

<!-- GoReleaser will automatically populate the changelog here -->

## Installation

### Binary Download
Download the appropriate binary for your platform from the assets below.

#### Linux
```bash
# AMD64
curl -LO https://github.com/jtzemp/dogfetch/releases/download/{{ .Tag }}/dogfetch_{{ .Version }}_Linux_x86_64.tar.gz
tar xzf dogfetch_{{ .Version }}_Linux_x86_64.tar.gz
sudo mv dogfetch /usr/local/bin/

# ARM64
curl -LO https://github.com/jtzemp/dogfetch/releases/download/{{ .Tag }}/dogfetch_{{ .Version }}_Linux_arm64.tar.gz
tar xzf dogfetch_{{ .Version }}_Linux_arm64.tar.gz
sudo mv dogfetch /usr/local/bin/
```

#### macOS
```bash
# Intel
curl -LO https://github.com/jtzemp/dogfetch/releases/download/{{ .Tag }}/dogfetch_{{ .Version }}_Darwin_x86_64.tar.gz
tar xzf dogfetch_{{ .Version }}_Darwin_x86_64.tar.gz
sudo mv dogfetch /usr/local/bin/

# Apple Silicon
curl -LO https://github.com/jtzemp/dogfetch/releases/download/{{ .Tag }}/dogfetch_{{ .Version }}_Darwin_arm64.tar.gz
tar xzf dogfetch_{{ .Version }}_Darwin_arm64.tar.gz
sudo mv dogfetch /usr/local/bin/
```

#### Windows
Download the zip file from assets below, extract, and add to your PATH.

### Go Install
```bash
go install github.com/jtzemp/dogfetch@{{ .Tag }}
```

## Verification

Verify the download with checksums:
```bash
# Download checksums
curl -LO https://github.com/jtzemp/dogfetch/releases/download/{{ .Tag }}/checksums.txt

# Verify (Linux/macOS)
shasum -a 256 -c checksums.txt

# Check version
dogfetch --version
```

## Documentation

See the [README](https://github.com/jtzemp/dogfetch#readme) for usage instructions.

---

**Full Changelog**: https://github.com/jtzemp/dogfetch/compare/{{ .PreviousTag }}...{{ .Tag }}
