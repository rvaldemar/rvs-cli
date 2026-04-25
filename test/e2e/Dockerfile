# End-to-end smoke test: install rvs from a fresh container, log in with
# a token from env, and run a non-interactive command. Used by the
# cli-e2e workflow on a release; can also be run manually:
#
#   docker build -t rvs-e2e -f test/e2e/Dockerfile .
#   docker run --rm \
#     -e RVS_TOKEN=... \
#     -e RVS_API_BASE=https://agents.rvs.solutions \
#     rvs-e2e
#
# The image is intentionally minimal — alpine + curl/ca-certificates only —
# so the test mirrors what an end user gets from the published install.sh.

FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl bash sudo

RUN adduser -D -s /bin/bash tester && \
    echo "tester ALL=(ALL) NOPASSWD: /usr/bin/install" >> /etc/sudoers

USER tester
WORKDIR /home/tester

COPY test/e2e/run.sh /home/tester/run.sh
ENTRYPOINT ["bash", "/home/tester/run.sh"]
