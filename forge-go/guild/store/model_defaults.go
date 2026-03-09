package store

func ensureJSONB(m *JSONB) {
	if *m == nil {
		*m = JSONB{}
	}
}

func ensureJSONBList(l *JSONBList) {
	if *l == nil {
		*l = JSONBList{}
	}
}

func ensureJSONBStringList(l *JSONBStringList) {
	if *l == nil {
		*l = JSONBStringList{}
	}
}
