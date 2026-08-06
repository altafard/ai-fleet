Analyze this repository and determine the container environment needed to build and test it.

Reply with ONLY a JSON object — no prose, no markdown fences — with exactly these fields:

{
  "base_image": "<image>",
  "packages": ["<apt package>"],
  "env": {"<NAME>": "<value>"}
}

Rules:
- "base_image" MUST be an official Docker Hub image based on Debian: apt-get and curl must work inside it. Never use alpine or any musl-based image.
- Prefer the official language image matching this project's toolchain and version (from go.mod, .tool-versions, package.json engines, rust-toolchain.toml, etc.), e.g. "golang:1.26-bookworm".
- "packages": apt package names required to build or test this project that are NOT already present in the base image. Use an empty array when none are needed.
- "env": only environment variables genuinely required to build or test this project. Use an empty object when none are needed.
- Do not include Claude Code, git, or curl in "packages" — the image build installs and configures those separately.
