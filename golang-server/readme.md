# Runnig the backend

- Setup direnv
- create .envrc file
- add following to .direnv

  ```
  export DEEPGRAM_KEY=XXX
  export OPENAI_API_KEY=XXX

  export AWS_ACCESS_KEY_ID="test"
  export AWS_SECRET_ACCESS_KEY="test"
  export AWS_DEFAULT_REGION="us-east-1"
  ```

- Add gobin to PATH by adding following to `~/.zshrc` or `~/.bashrc`

  ```
  export PATH="$(go env GOBIN):${PATH}"
  export PATH="$(go env GOPATH)/bin:${PATH}"
  ```

- Install docker

- `make install-deps`
- `make build`
- `make lint`
- `make run`
