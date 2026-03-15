// naming_export_test.go exposes unexported naming helpers for use by external test packages.
package messaging

// SanitizeForTest exposes sanitize for testing from external packages.
func SanitizeForTest(name string) string { return sanitize(name) }

// JsSubjectForTest exposes jsSubject for testing from external packages.
func JsSubjectForTest(topic string) string { return jsSubject(topic) }

// StreamNameForTest exposes streamName for testing from external packages.
func StreamNameForTest(topic string) string { return streamName(topic) }

// KvBucketNameForTest exposes kvBucketName for testing from external packages.
func KvBucketNameForTest(ns string) string { return kvBucketName(ns) }
