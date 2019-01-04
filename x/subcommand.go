/*
 * Copyright 2017-2018 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package x

import (
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type SubCommand struct {
	Cmd  *cobra.Command
	Conf *viper.Viper

	EnvPrefix string
}

func (s SubCommand) GetStringSliceP(name, shorthand string, def []string) (v []string) {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetStringSlice(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetStringSlice(shorthand)
	}
	return def
}

func (s SubCommand) GetStringP(name, shorthand, def string) string {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetString(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetString(shorthand)
	}
	return def
}

func (s SubCommand) GetFloat64P(name, shorthand string, def float64) float64 {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetFloat64(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetFloat64(shorthand)
	}
	return def

}

func (s SubCommand) GetIntP(name, shorthand string, def int) int {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetInt(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetInt(shorthand)
	}
	return def
}

func (s SubCommand) GetInt64P(name, shorthand string, def int64) int64 {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetInt64(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetInt64(shorthand)
	}
	return def

}

func (s SubCommand) GetDurationP(name, shorthand string, def time.Duration) time.Duration {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetDuration(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetDuration(shorthand)
	}
	return def

}

func (s SubCommand) GetStringMapP(name, shorthand string, def map[string]interface{}) map[string]interface{} {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetStringMap(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetStringMap(shorthand)
	}
	return def
}

func (s SubCommand) GetStringMapStringP(name, shorthand string, def map[string]string) map[string]string {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetStringMapString(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetStringMapString(shorthand)
	}
	return def
}

func (s SubCommand) GetStringMapStringSliceP(name, shorthand string, def map[string][]string) map[string][]string {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetStringMapStringSlice(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetStringMapStringSlice(shorthand)
	}
	return def
}

func (s SubCommand) GetBoolP(name, shorthand string, def bool) bool {
	if ok := s.Conf.IsSet(name); ok {
		return s.Conf.GetBool(name)
	}
	if ok := s.Conf.IsSet(shorthand); ok {
		return s.Conf.GetBool(shorthand)
	}
	return def
}
