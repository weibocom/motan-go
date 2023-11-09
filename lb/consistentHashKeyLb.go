package lb

import (
	"errors"
	"github.com/buraksezer/consistent"
	"github.com/cespare/xxhash/v2"
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	"math/rand"
	"strconv"
)

const (
	// config keys
	PartSizeKey = "consistentHashKey.partSize"
	LoadKey     = "consistentHashKey.load"
	ReplicaKey  = "consistentHashKey.replica"

	// default values
	DefaultPartSizeAddend = 271
	DefaultReplica        = 10
	DefaultMinLoad        = 1.1
)

var BuildConsistentHashFailError = errors.New("build consistent hash fail")

type ConsistentHashLB struct {
	url          *motan.URL
	endpoints    []motan.EndPoint
	cHash        *consistent.Consistent
	lastPartSize int
	load         float64
	replica      int
}

type member struct {
	Key      string // Key needs to be generated at build time
	Endpoint motan.EndPoint
}

func (m *member) String() string {
	return m.Key
}

type hasher struct{}

func (h hasher) Sum64(data []byte) uint64 {
	return xxhash.Sum64(data)
}

func (c *ConsistentHashLB) OnRefresh(endpoints []motan.EndPoint) {
	if len(endpoints) == 1 {
		c.endpoints = endpoints
		c.cHash = nil
		return
	}
	ok := c.buildConsistent(endpoints)
	if !ok {
		vlog.Errorf("ConsistentHashLB OnRefresh failed, endpoints not update. endpoints size:%d\n", len(endpoints))
		return
	}
	c.endpoints = endpoints
}

func (c *ConsistentHashLB) Select(request motan.Request) motan.EndPoint {
	if len(c.endpoints) == 1 {
		return c.endpoints[0]
	}
	key := request.GetAttachment(motan.ConsistentHashKey)
	var endpoint motan.EndPoint
	if key != "" { // Use consistent hashing when hash key is not empty
		endpoint = c.cHash.LocateKey([]byte(key)).(*member).Endpoint
	}
	if endpoint == nil || !endpoint.IsAvailable() { // When the hash key is empty or the hash endpoint is unavailable, the endpoint is randomly selected.
		_, endpoint = SelectOneAtRandom(c.endpoints)
	}
	return endpoint
}

func (c *ConsistentHashLB) SelectArray(request motan.Request) []motan.EndPoint {
	if len(c.endpoints) > MaxSelectArraySize {
		key := request.GetAttachment(motan.ConsistentHashKey)
		if key != "" {
			members, err := c.cHash.GetClosestN([]byte(key), MaxSelectArraySize)
			if err != nil {
				vlog.Warningf("ConsistentHashLB SelectArray failed, key:%s, err:%v\n", key, err)
			} else {
				endpoints := make([]motan.EndPoint, 0, len(members))
				for _, m := range members {
					if m.(*member).Endpoint.IsAvailable() {
						endpoints = append(endpoints, m.(*member).Endpoint)
					}
				}
				return endpoints
			}
		}
	}
	return SelectArrayFromIndex(c.endpoints, rand.Intn(len(c.endpoints)))
}

func (c *ConsistentHashLB) SetWeight(weight string) {
}

func (c *ConsistentHashLB) buildConsistent(endpoints []motan.EndPoint) bool {
	//Calculate partSize
	partSize := 0
	if c.lastPartSize == 0 { // first build
		partSize = int(c.url.GetIntValue(PartSizeKey, 0))
	} else { // Try not to change the size when building again
		partSize = c.lastPartSize
	}
	if partSize < len(endpoints)*2 { // Recalculate size when conditions are not met
		partSize = len(endpoints)*5 + DefaultPartSizeAddend
	}

	// Calculate load on first build
	if c.load == 0 {
		load, err := strconv.ParseFloat(c.url.GetParam(LoadKey, "0"), 64)
		if err != nil || load < DefaultMinLoad {
			load = DefaultMinLoad
		}
		c.load = load
	}

	// Calculate replica on first build
	if c.replica == 0 {
		replica := int(c.url.GetIntValue(ReplicaKey, 0))
		if replica <= 0 {
			if len(endpoints) < 100 {
				replica = DefaultReplica * 2
			} else {
				replica = DefaultReplica
			}
		}
		c.replica = replica
	}
	// Build member list
	members := make([]consistent.Member, len(endpoints))
	for i := 0; i < len(endpoints); i++ {
		members[i] = &member{Key: endpoints[i].GetURL().GetAddressStr(), Endpoint: endpoints[i]}
	}
	var cHash *consistent.Consistent
	var err error
	count := 0
	load := c.load
	for {
		cHash, err = c.buildConsistent0(members, partSize, c.replica, load)
		if err == nil {
			break
		}
		// Increase load when build failed
		load += 0.1
		count++
		if count > 20 { // Try 20 times at most, if it still fails, give up
			vlog.Errorf("build consistent hash ring failed after maximum retries. part size:%d, load:%.2f, replica:%d, member:%d\n", partSize, load, c.replica, len(endpoints))
			return false
		}
	}
	c.cHash = cHash
	c.lastPartSize = partSize
	if load != c.load {
		c.load = load
	}
	vlog.Infof("build consistent hash ring, part size:%d, load:%.2f, replica:%d, member:%d", c.lastPartSize, c.load, c.replica, len(endpoints))
	return true
}

func (c *ConsistentHashLB) buildConsistent0(members []consistent.Member, partSize int, replica int, load float64) (cHash *consistent.Consistent, err error) {
	defer func() {
		if r := recover(); r != nil {
			vlog.Warningf("build consistent hash ring failed with part size:%d, load:%.2f, replica:%d, member:%d, err:%v\n", partSize, load, replica, len(members), r)
			err = BuildConsistentHashFailError
		}
	}()
	cfg := consistent.Config{
		PartitionCount:    partSize,
		ReplicationFactor: replica,
		Load:              load,
		Hasher:            hasher{},
	}
	cHash = consistent.New(members, cfg)
	return cHash, nil
}
