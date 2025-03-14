/* Copyright 2022 Zinc Labs Inc. and Contributors
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

package core

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/blugelabs/bluge"
	"github.com/blugelabs/bluge/analysis"

	"github.com/zinclabs/zinc/pkg/bluge/directory"
	"github.com/zinclabs/zinc/pkg/config"
	"github.com/zinclabs/zinc/pkg/meta"
	"github.com/zinclabs/zinc/pkg/metadata"
)

var indexNameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func CheckIndexName(name string) error {
	if name == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if strings.HasPrefix(name, "_") {
		return fmt.Errorf("index name cannot start with _")
	}
	if !indexNameRe.Match([]byte(name)) {
		return fmt.Errorf("index name [%s] is invalid, just accept [a-zA-Z0-9_.-]", name)
	}
	return nil
}

// NewIndex creates an instance of a physical zinc index that can be used to store and retrieve data.
func NewIndex(name, storageType string) (*Index, error) {
	if err := CheckIndexName(name); err != nil {
		return nil, err
	}

	if storageType == "" {
		storageType = "disk"
	}

	index := new(Index)
	index.Name = name
	index.StorageType = storageType
	index.ShardNum = 1
	index.CreateAt = time.Now()

	// use template
	if err := index.UseTemplate(); err != nil {
		return nil, err
	}

	// load WAL
	if err := index.OpenWAL(); err != nil {
		return nil, err
	}

	// init shards writer
	for i := int64(0); i < index.ShardNum; i++ {
		index.Shards = append(index.Shards, &meta.IndexShard{ID: i})
	}

	return index, nil
}

// LoadIndexWriter load the index writer from the storage
func OpenIndexWriter(name string, storageType string, defaultSearchAnalyzer *analysis.Analyzer, timeRange ...int64) (*bluge.Writer, error) {
	cfg := getOpenConfig(name, storageType, defaultSearchAnalyzer, timeRange...)
	return bluge.OpenWriter(cfg)
}

func getOpenConfig(name string, storageType string, defaultSearchAnalyzer *analysis.Analyzer, timeRange ...int64) bluge.Config {
	var dataPath string
	var cfg bluge.Config
	switch storageType {
	case "s3":
		dataPath = config.Global.S3.Bucket
		cfg = directory.GetS3Config(dataPath, name, timeRange...)
	case "minio":
		dataPath = config.Global.MinIO.Bucket
		cfg = directory.GetMinIOConfig(dataPath, name, timeRange...)
	default:
		dataPath = config.Global.DataPath
		cfg = directory.GetDiskConfig(dataPath, name, timeRange...)
	}
	if defaultSearchAnalyzer != nil {
		cfg.DefaultSearchAnalyzer = defaultSearchAnalyzer
	}
	return cfg
}

// storeIndex stores the index to metadata
func StoreIndex(index *Index) error {
	// store index
	if err := storeIndex(index); err != nil {
		return err
	}
	// cache index
	ZINC_INDEX_LIST.Add(index)
	return nil
}

func storeIndex(index *Index) error {
	index.lock.Lock()
	defer index.lock.Unlock()

	if index.Settings == nil {
		index.Settings = new(meta.IndexSettings)
	}
	if index.Analyzers == nil {
		index.Analyzers = make(map[string]*analysis.Analyzer)
	}
	if index.Mappings == nil {
		// set default mappings
		index.Mappings = meta.NewMappings()
		index.Mappings.SetProperty(meta.TimeFieldName, meta.NewProperty("date"))
	}

	index.UpdateAt = time.Now()
	err := metadata.Index.Set(index.Name, index.Index)
	if err != nil {
		return fmt.Errorf("core.StoreIndex: index: %s, error: %s", index.Name, err.Error())
	}

	return nil
}

func GetIndex(name string) (*Index, bool) {
	return ZINC_INDEX_LIST.Get(name)
}

func GetOrCreateIndex(name, storageType string) (*Index, bool, error) {
	return ZINC_INDEX_LIST.GetOrCreate(name, storageType)
}
