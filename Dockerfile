FROM golang:1.17 as build

WORKDIR /app
COPY . ./

RUN go install ./...

FROM ubuntu:focal as app
WORKDIR /app

RUN apt-get update && \
    apt-get dist-upgrade -y && \
    apt-get install -y curl gnupg2

RUN curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/focal.gpg | apt-key add - && \
    curl -fsSL https://pkgs.tailscale.com/stable/ubuntu/focal.list > /etc/apt/sources.list.d/tailscale.list && \
    apt-get update && \
    apt-get install -y tailscale

# Copy binary to production image
COPY --from=build /app/run.sh /app/run.sh
COPY --from=build /go/bin/alertr /app/alertr

# Run on container startup.
CMD ["/app/run.sh"]
