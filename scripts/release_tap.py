"""Write the Homebrew tap formula for an english-teacher release.

The whole formula is rendered from the template below, so the first release
creates it and later releases rewrite it. Used by scripts/release.sh; it does
not commit or push.

Usage: release_tap.py PATH|- URL VERSION SHA256   ("-" prints to stdout)
"""
import re
import sys

TEMPLATE = '''class EnglishTeacher < Formula
  desc "Personal English teacher in the terminal: spaced review, reading, writing, role-plays"
  homepage "https://github.com/kevnnard/english-teacher"
  url "{url}"
  version "{version}"
  sha256 "{sha}"
  license "PolyForm-Strict-1.0.0"
  head "https://github.com/kevnnard/english-teacher.git", branch: "main"

  depends_on "go" => :build

  def install
    ldflags = "-s -w -X main.version=#{{version}}"
    system "go", "build", *std_go_args(ldflags:, output: bin/"teacher"), "./cmd/teacher"
  end

  def caveats
    <<~EOS
      Review, Fluency and the vocabulary quiz work offline.
      Learn, Write, Talk and the grammar quiz use the `pi` CLI (Gentle Shell);
      see `teacher models`.
    EOS
  end

  test do
    assert_match version.to_s, shell_output("#{{bin}}/teacher --version")
  end
end
'''


def render(url, version, sha):
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        raise SystemExit(f"bad version: {version}")
    if not re.fullmatch(r"[0-9a-f]{64}", sha):
        raise SystemExit(f"bad sha256: {sha}")
    return TEMPLATE.format(url=url, version=version, sha=sha)


def main():
    path, url, version, sha = sys.argv[1:5]
    body = render(url, version, sha)
    if path == "-":
        sys.stdout.write(body)
    else:
        with open(path, "w") as f:
            f.write(body)


if __name__ == "__main__":
    main()
