# Bench task base: node 22 + opencode baked at build time. The Artifactory npm token enters as a
# BuildKit secret (a one-line temp file, value-only — never the user's ~/.npmrc, never a layer).
FROM registry.example.com/node:22-bookworm
RUN --mount=type=secret,id=npmtoken,target=/run/secrets/tok \
    printf 'registry=https://artifactory.example.com/artifactory/api/npm/npm/\nstrict-ssl=false\n//artifactory.example.com/artifactory/api/npm/npm/:_authToken=%s\n' "$(cat /run/secrets/tok)" > /root/.npmrc \
 && npm i -g opencode-ai \
 && npm cache add @ai-sdk/openai-compatible \
 && rm -f /root/.npmrc \
 && printf 'registry=https://artifactory.example.com/artifactory/api/npm/npm/\nstrict-ssl=false\n//artifactory.example.com/artifactory/api/npm/npm/:_authToken=${NPM_AUTH_TOKEN}\n' > /root/.npmrc

