"""Upload binary + installer to the VPS and run remote_install.sh.

The web UI is embedded in the binary, so only two files are uploaded.
Credentials come from environment variables (never hardcode):
  AINOVEL_SSH_HOST, AINOVEL_SSH_USER, AINOVEL_SSH_PASSWORD
"""
import os
import sys
from pathlib import Path

import paramiko


def main() -> int:
    host = os.environ.get("AINOVEL_SSH_HOST")
    user = os.environ.get("AINOVEL_SSH_USER", "root")
    password = os.environ.get("AINOVEL_SSH_PASSWORD")
    if not host or not password:
        print("Thiếu AINOVEL_SSH_HOST / AINOVEL_SSH_PASSWORD trong môi trường", file=sys.stderr)
        return 1

    binary = Path("deploy/ainovel-cli-linux-amd64")
    installer = Path("deploy/remote_install.sh")
    for f in (binary, installer):
        if not f.is_file():
            print(f"Không tìm thấy {f} — build trước khi deploy", file=sys.stderr)
            return 1

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(host, username=user, password=password, timeout=30,
                   allow_agent=False, look_for_keys=False)
    try:
        sftp = client.open_sftp()
        sftp.put(str(binary), "/tmp/ainovel-cli-linux-amd64")
        sftp.put(str(installer), "/tmp/remote_install.sh")
        sftp.close()

        _, stdout, stderr = client.exec_command(
            "chmod +x /tmp/ainovel-cli-linux-amd64 /tmp/remote_install.sh"
            " && bash /tmp/remote_install.sh",
            timeout=180,
        )
        out = stdout.read().decode("utf-8", "replace")
        err = stderr.read().decode("utf-8", "replace")
        Path("deploy/remote_install.out").write_text(out + "\nSTDERR\n" + err, encoding="utf-8")
        print("DONE", len(out), len(err))
        print(out[-2000:])
    finally:
        client.close()
    return 0


if __name__ == "__main__":
    sys.exit(main())
