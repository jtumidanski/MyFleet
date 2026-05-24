FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY packages ./packages
COPY apps/media-service ./apps/media-service
RUN cd apps/media-service && go build -o /out/media-service ./cmd

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/media-service /media-service
EXPOSE 8080
ENTRYPOINT ["/media-service"]
