/**
 * server.ParsePage defaults page[size] to 25 and hard-caps it at 100
 * (packages/shared-go/server/pagination.go:25,29). Requesting more is silently
 * clamped, so 100 is the largest page a client can actually get.
 */
export const RECORD_PAGE_SIZE = 100;
