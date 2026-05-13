# Plan: write `gh-optivem.yaml` once, inside the scaffolded repo only

All code, test, and doc steps are committed. Only end-to-end verification remains — it needs the operator's GitHub creds + a live scaffold run, so it can't be exercised in CI or by an agent.

- [ ] Manual verification — ⏳ Deferred: requires operator-side `gh optivem init` run with real credentials.

  Reproduce the original failure scenario and the success path:

  ```sh
  cd $(mktemp -d)
  gh optivem init --owner valentinajemuovic --repo page-turner-XX --system-name "Page Turner" \
      --arch multitier --repo-strategy multi-repo ...
  # Force or wait for a step to fail (e.g. point at a bad sonar config).
  ls "$PWD"   # expect: empty / no gh-optivem.yaml
  ```

  Also verify the success path: a clean `init` run leaves `gh-optivem.yaml` inside the scaffolded repo on the remote (unchanged) and nothing in the CWD (new behavior).
