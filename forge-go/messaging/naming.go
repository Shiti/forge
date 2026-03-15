package messaging

import "strings"

// sanitize replaces NATS-reserved characters with underscores, matching the
// Python implementation in nats/messaging/message_store.py.
func sanitize(name string) string {
	r := strings.NewReplacer(":", "_", ".", "_", "$", "_")
	return r.Replace(name)
}

// jsSubject returns the JetStream subject for a (namespaced) topic.
// Matches Python: "persist." + sanitize(topic)
func jsSubject(topic string) string {
	return "persist." + sanitize(topic)
}

// streamName returns the JetStream stream name for a (namespaced) topic.
// Matches Python: "MSGS_" + sanitize(topic)
func streamName(topic string) string {
	return "MSGS_" + sanitize(topic)
}

// kvBucketName returns the KV bucket name for a namespace.
// Matches Python: "msg-cache-" + sanitize(namespace)
func kvBucketName(ns string) string {
	return "msg-cache-" + sanitize(ns)
}
