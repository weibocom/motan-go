package cluster

import (
	"fmt"
	motan "github.com/weibocom/motan-go/core"
	vlog "github.com/weibocom/motan-go/log"
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
}

func (fr filterRule) IsMatch(refer motan.EndPoint) bool {
	for _, prefix := range fr.prefixes {
		if strings.HasPrefix(refer.GetURL().Host, prefix) {
			if fr.mode == FilterModeExclude {
				vlog.Infof("filter refer: %s, rule: %s, mode: %s", refer.GetURL().GetIdentity(), prefix, fr.mode)
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
		rule := filterRule{
			mode:     config.Mode,
			prefixes: motan.TrimSplit(config.Rule, ","),
		}
		if config.Mode == FilterModeExclude {
			excludeRules = append(excludeRules, rule)
		} else {
			includeRules = append(includeRules, rule)
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
