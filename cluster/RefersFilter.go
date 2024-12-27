package cluster

import (
	"fmt"
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
	"net"
	"strings"
)

const (
	FilterModeInclude = "include"
	FilterModeExclude = "exclude"
)

type RefersFilterConfigList []RefersFilterConfig

type RefersFilterConfig struct {
	Group   string `json:"group"`
	Service string `json:"service"`
	Mode    string `json:"mode"`
	Rule    string `json:"rule"`
}

func (list RefersFilterConfigList) Verify() error {
	for _, filter := range list {
		if filter.Rule == "" {
			return fmt.Errorf("exist empty rule in filter config")
		}
		if filter.Mode != FilterModeInclude && filter.Mode != FilterModeExclude {
			return fmt.Errorf("invalid mode(%s) for rule(%s)", filter.Mode, filter.Rule)
		}
	}
	return nil
}

func (list RefersFilterConfigList) ParseRefersFilters(clusterURL *motan.URL) RefersFilter {
	var rules []RefersFilterConfig
	for _, filter := range list {
		if filter.Group != "" && filter.Group != clusterURL.Group {
			continue
		}
		if filter.Service != "" && filter.Service != clusterURL.Path {
			continue
		}
		rules = append(rules, filter)
		vlog.Infof("add refer filter for group: %s, service: %s, rule: %s, mode: %s", clusterURL.Group, clusterURL.Path, filter.Rule, filter.Mode)
	}
	if len(rules) == 0 {
		return nil
	}
	return NewDefaultRefersFilter(rules)
}

type RefersFilter interface {
	Filter([]motan.EndPoint) []motan.EndPoint
}

type filterRule struct {
	mode     string
	prefixes []string
	subnets  []*net.IPNet
}

func (fr filterRule) IsMatch(refer motan.EndPoint) bool {
	for _, prefix := range fr.prefixes {
		if strings.HasPrefix(refer.GetURL().Host, prefix) {
			if fr.mode == FilterModeExclude {
				vlog.Infof("filter refer: %s, prefix rule: %s, mode: %s", refer.GetURL().GetIdentity(), prefix, fr.mode)
			}
			return true
		}
	}
	if len(fr.subnets) == 0 {
		return false
	}
	ip := net.ParseIP(refer.GetURL().Host)
	if ip == nil {
		vlog.Errorf("invalid refer ip: %s", refer.GetURL().Host)
		return false
	}
	for _, ipNet := range fr.subnets {
		if ipNet.Contains(ip) {
			if fr.mode == FilterModeExclude {
				vlog.Infof("filter refer: %s, subnet rule: %s, mode: %s", refer.GetURL().GetIdentity(), ipNet.String(), fr.mode)
			}
			return true
		}
	}
	return false
}

type DefaultRefersFilter struct {
	excludeRules []filterRule
	includeRules []filterRule
}

func NewDefaultRefersFilter(filterConfig []RefersFilterConfig) *DefaultRefersFilter {
	var includeRules []filterRule
	var excludeRules []filterRule
	for _, config := range filterConfig {
		rules := motan.TrimSplit(config.Rule, ",")
		fr := filterRule{
			mode:     config.Mode,
			prefixes: []string{},
			subnets:  []*net.IPNet{},
		}
		for _, item := range rules {
			if strings.Contains(item, "/") {
				_, subnet, err := net.ParseCIDR(item)
				if err != nil {
					vlog.Errorf("invalid subnet rule: %s", item)
					continue
				}
				fr.subnets = append(fr.subnets, subnet)
			} else {
				fr.prefixes = append(fr.prefixes, item)
			}
		}
		if config.Mode == FilterModeExclude {
			excludeRules = append(excludeRules, fr)
		} else {
			includeRules = append(includeRules, fr)
		}
	}
	return &DefaultRefersFilter{includeRules: includeRules, excludeRules: excludeRules}
}

func (f *DefaultRefersFilter) Filter(refers []motan.EndPoint) []motan.EndPoint {
	var newRefers []motan.EndPoint
	for _, refer := range refers {
		// discard refer if hit an exclude rule
		excludeRefer := false
		for _, excludeRule := range f.excludeRules {
			excludeRefer = excludeRule.IsMatch(refer)
			if excludeRefer {
				break
			}
		}
		if excludeRefer {
			continue
		}
		// retained refer if hit an include rule
		var includeRefer bool
		if len(f.includeRules) == 0 {
			includeRefer = true
		}
		for _, includeRule := range f.includeRules {
			includeRefer = includeRule.IsMatch(refer)
			if includeRefer {
				break
			}
		}
		if includeRefer {
			newRefers = append(newRefers, refer)
		} else {
			vlog.Infof("no include rule hit. filter refer: %s", refer.GetURL().GetIdentity())
		}
	}
	return newRefers
}
