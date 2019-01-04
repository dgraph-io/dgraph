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
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestSubCommand(t *testing.T) {

	config := `{ 
		"a": "a.string" ,
		"string_short" : "b.string",
		"d" : 13,
		"int_short":15,

		"g": "true",
		"bool1_short":true,

		"j":"false",
		"bool2_short":false,

		"m":"true",
		"bool3_short":true,

		"p":"false",
		"bool4_short":false
	}`

	fname := tmpConfFile(config)
	defer func() {
		os.Remove(fname)
	}()
	s := initSubCommand(fname)
	flag := s.Cmd.Flags()
	defaultFlagString := "def_string"
	defaultFlagInt := 19
	defaultFlagBoolFalse := false
	defaultFlagBoolTrue := true

	flag.StringP("string_full", "a", defaultFlagString, "")
	flag.StringP("string_short", "b", defaultFlagString, "")
	flag.StringP("string_default", "c", defaultFlagString, "")

	flag.IntP("int_full", "d", defaultFlagInt, "")
	flag.IntP("int_short", "e", defaultFlagInt, "")
	flag.IntP("int_default", "f", defaultFlagInt, "")

	flag.BoolP("bool1_full", "g", defaultFlagBoolFalse, "")
	flag.BoolP("bool1_short", "h", defaultFlagBoolFalse, "")
	flag.BoolP("bool1_default", "i", defaultFlagBoolFalse, "")

	flag.BoolP("bool2_full", "j", defaultFlagBoolFalse, "")
	flag.BoolP("bool2_short", "k", defaultFlagBoolFalse, "")
	flag.BoolP("bool2_default", "l", defaultFlagBoolFalse, "")

	flag.BoolP("bool3_full", "m", defaultFlagBoolTrue, "")
	flag.BoolP("bool3_short", "n", defaultFlagBoolTrue, "")
	flag.BoolP("bool3_default", "o", defaultFlagBoolTrue, "")

	flag.BoolP("bool4_full", "p", defaultFlagBoolTrue, "")
	flag.BoolP("bool4_short", "q", defaultFlagBoolTrue, "")
	flag.BoolP("bool4_default", "r", defaultFlagBoolTrue, "")

	if v := s.GetStringP("string_full", "a", defaultFlagString); v != "a.string" {
		t.Fatalf("'%v' should be equal 'a.string'", v)
	}

	if v := s.GetStringP("string_short", "b", defaultFlagString); v != "b.string" {
		t.Fatalf("should be equal 'b.string'")
	}
	if v := s.GetStringP("string_default", "c", defaultFlagString); v != defaultFlagString {
		t.Fatalf("should be equal '%v'", defaultFlagString)
	}

	if v := s.GetIntP("int_full", "d", defaultFlagInt); v != 13 {
		t.Fatalf("should be equal '13'")
	}

	if v := s.GetIntP("int_short", "e", defaultFlagInt); v != 15 {
		t.Fatalf("should be equal '15'")
	}
	if v := s.GetIntP("int_default", "f", defaultFlagInt); v != defaultFlagInt {
		t.Fatalf("should be equal '%v'", defaultFlagInt)
	}

	if v := s.GetBoolP("bool1_full", "g", defaultFlagBoolFalse); v != true {
		t.Fatalf("should be equal 'true'")
	}

	if v := s.GetBoolP("bool1_short", "h", defaultFlagBoolFalse); v != true {
		t.Fatalf("should be equal 'true'")
	}
	if v := s.GetBoolP("bool1_default", "i", defaultFlagBoolFalse); v != false {
		t.Fatalf("should be equal '%v'", defaultFlagBoolFalse)
	}

	if v := s.GetBoolP("bool2_full", "j", defaultFlagBoolFalse); v != false {
		t.Fatalf("should be equal 'false'")
	}

	if v := s.GetBoolP("bool2_short", "k", defaultFlagBoolFalse); v != false {
		t.Fatalf("should be equal 'false'")
	}
	if v := s.GetBoolP("bool2_default", "l", defaultFlagBoolFalse); v != false {
		t.Fatalf("should be equal '%v'", defaultFlagBoolFalse)
	}

	if v := s.GetBoolP("bool3_full", "m", defaultFlagBoolTrue); v != true {
		t.Fatalf("should be equal 'true'")
	}

	if v := s.GetBoolP("bool3_short", "n", defaultFlagBoolTrue); v != true {
		t.Fatalf("should be equal 'true'")
	}
	if v := s.GetBoolP("bool3_default", "o", defaultFlagBoolTrue); v != true {
		t.Fatalf("should be equal '%v'", defaultFlagBoolTrue)
	}

	if v := s.GetBoolP("bool4_full", "p", defaultFlagBoolTrue); v != false {
		t.Fatalf("should be equal 'false'")
	}

	if v := s.GetBoolP("bool4_short", "q", defaultFlagBoolTrue); v != false {
		t.Fatalf("should be equal 'false'")
	}
	if v := s.GetBoolP("bool4_default", "r", defaultFlagBoolTrue); v != true {
		t.Fatalf("should be equal '%v'", defaultFlagBoolTrue)
	}

}

func tmpConfFile(data string) string {
	fname := fmt.Sprintf("%s/subcommand_%f.json", os.TempDir(), random())
	f, err := os.OpenFile(fname, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	defer f.Close()
	if err != nil {
		panic(err)
	}
	_, err = f.WriteString(data)
	if err != nil {
		panic(err)
	}

	var vmap map[string]interface{}
	if err = json.Unmarshal([]byte(data), &vmap); err != nil { //check json is ok
		panic(fmt.Errorf("you should check json format is ok"))
	}
	return f.Name()
}

func random() float64 {
	source := rand.NewSource(time.Now().UnixNano() + 20033332)
	r := rand.New(source)
	return r.Float64()
}

func initSubCommand(filepath string) (s SubCommand) {
	if _, err := os.Stat(filepath); err != nil {
		panic(fmt.Errorf("[%s] not exists.", filepath))
	}
	s.Cmd = &cobra.Command{
		Use: "test-subcommand",
	}
	s.EnvPrefix = "TEST_SUBCOMMAND"
	s.Conf = viper.New()
	s.Conf.AutomaticEnv()
	s.Conf.SetEnvPrefix(s.EnvPrefix)
	s.Conf.SetConfigFile(filepath)
	s.Conf.ReadInConfig()
	return s
}
