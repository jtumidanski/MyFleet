FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY packages ./packages
COPY apps/auth-service ./apps/auth-service
RUN cd apps/auth-service && go build -o /out/auth-service ./cmd

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/auth-service /auth-service
EXPOSE 8080
ENTRYPOINT ["/auth-service"]
