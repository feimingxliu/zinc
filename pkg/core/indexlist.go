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
	"sort"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/zinclabs/zinc/pkg/meta"
	"github.com/zinclabs/zinc/pkg/metadata"
)

var ZINC_INDEX_LIST IndexList

type IndexList struct {
	Indexes map[string]*Index
	lock    sync.RWMutex
}

func init() {
	// check version
	version, _ := metadata.KV.Get("version")
	if version == nil {
		err := metadata.KV.Set("version", []byte(meta.Version))
		if err != nil {
			fmt.Println("error:", err)
		}
		// } else {
		// version := string(version)
		// if version != meta.Version {
		// 	log.Error().Msgf("Version mismatch, loading data from old version %s", version)
		// 	if err := upgrade.Do(version); err != nil {
		// 		log.Fatal().Err(err).Msg("Failed to upgrade")
		// 	}
		// }
	}

	// start loading index
	ZINC_INDEX_LIST.Indexes = make(map[string]*Index)
	if err := LoadZincIndexesFromMetadata(); err != nil {
		log.Error().Err(err).Msg("Error loading index")
	}
}

func (t *IndexList) Add(index *Index) {
	t.lock.Lock()
	t.Indexes[index.Name] = index
	t.lock.Unlock()
}

func (t *IndexList) Get(name string) (*Index, bool) {
	t.lock.RLock()
	idx, ok := t.Indexes[name]
	t.lock.RUnlock()
	return idx, ok
}

func (t *IndexList) GetOrCreate(name, storageType string) (*Index, bool, error) {
	t.lock.RLock()
	idx, ok := t.Indexes[name]
	t.lock.RUnlock()
	if ok {
		return idx, true, nil
	}
	t.lock.Lock()
	defer t.lock.Unlock()
	// maybe someone else created it while we were waiting for the lock
	idx, ok = t.Indexes[name]
	if ok {
		return idx, true, nil
	}
	// okay, let's create new index
	idx, err := NewIndex(name, storageType)
	if err != nil {
		return nil, false, err
	}
	if err = storeIndex(idx); err != nil {
		return nil, false, err
	}
	// cache it
	t.Indexes[idx.Name] = idx
	return idx, false, nil
}

func (t *IndexList) Delete(name string) {
	t.lock.Lock()
	if idx, ok := t.Indexes[name]; ok {
		if err := idx.Close(); err != nil {
			log.Error().Err(err).Msgf("Error Delete index[%s]", name)
		}
	}
	delete(t.Indexes, name)
	t.lock.Unlock()
}

func (t *IndexList) Len() int {
	t.lock.RLock()
	n := len(t.Indexes)
	t.lock.RUnlock()
	return n
}

func (t *IndexList) List() []*Index {
	t.lock.RLock()
	indexes := make([]*Index, 0, len(t.Indexes))
	for _, index := range t.Indexes {
		indexes = append(indexes, index)
	}
	t.lock.RUnlock()
	return indexes
}

func (t *IndexList) ListStat() []*Index {
	items := t.List()
	for _, index := range items {
		index.lock.Lock()
		size, _ := index.WAL.Len()
		if size == 1 {
			size = 0
		}
		index.WALSize = size
		index.lock.Unlock()
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].GetName() < items[j].GetName()
	})

	return items
}

func (t *IndexList) ListName() []string {
	items := t.List()
	sort.Slice(items, func(i, j int) bool {
		return items[i].GetName() < items[j].GetName()
	})

	names := make([]string, 0, len(items))
	for _, index := range items {
		names = append(names, index.GetName())
	}

	return names
}

func (t *IndexList) Close() error {
	t.lock.Lock()
	defer t.lock.Unlock()
	for _, index := range t.Indexes {
		if err := index.Close(); err != nil {
			return err
		}
	}
	return nil
}

// GC auto close unused indexes what unactive for a long time (10m)
func (t *IndexList) GC() error {
	return nil // TODO: implement GC
}
