FROM ubuntu:24.04
COPY .test-bin/shippo /usr/local/bin/shippo
COPY .test-bin/daemon.test /usr/local/bin/daemon.test
ENV SHIPPO_TEST_BINARY=/usr/local/bin/shippo
ENTRYPOINT ["/usr/local/bin/daemon.test", "-test.run=^TestIntegration", "-test.v", "-test.timeout=90s"]
