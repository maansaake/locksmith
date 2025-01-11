package queue

import "hash/fnv"

const MAX_HASH uint16 = 65535

func fnv1aHash(str string) uint16 {
	alg := fnv.New32a()
	alg.Write([]byte(str))
	//nolint:gosec
	return uint16(alg.Sum32() % 65535)
}
