FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY packages ./packages
COPY apps/fleet-service ./apps/fleet-service
RUN cd apps/fleet-service && go build -o /out/fleet-service ./cmd

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/fleet-service /fleet-service
EXPOSE 8080
ENTRYPOINT ["/fleet-service"]
