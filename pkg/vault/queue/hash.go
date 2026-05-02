package queue

import "hash/fnv"

const MAX_HASH uint16 = 65535

func fnv1aHash(str string) uint16 {
	alg := fnv.New32a()
	_, _ = alg.Write([]byte(str))

	//nolint:mnd // ignore
	return uint16(alg.Sum32() % 65535)
}
