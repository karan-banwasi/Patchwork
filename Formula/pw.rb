class Pw < Formula
  desc "Cross-platform CLI tool to check and manage package updates across multiple package managers"
  homepage "https://github.com/karan-banwasi/Patchwork"
  url "https://github.com/karan-banwasi/Patchwork/archive/refs/tags/v0.2.1.tar.gz"
  sha256 "ab8e0b0ba168f815b048b8dd80e28aa04c55ad360f7acd422f5bfd8a2089787e"
  license "MIT"
  head "https://github.com/karan-banwasi/Patchwork.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w -X github.com/karan-banwasi/patchwork/cmd.version=v#{version}"), "./cmd/pw"
    generate_completions_from_executable(bin/"pw", "completion")
  end

  test do
    assert_match "Patchwork", shell_output("#{bin}/pw --help")
    assert_match version.to_s, shell_output("#{bin}/pw version")
  end
end
