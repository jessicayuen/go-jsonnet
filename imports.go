/*
Copyright 2017 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package jsonnet

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
)

type ImportedData struct {
	err       error
	foundHere string
	content   string
}

type Importer interface {
	Import(codeDir string, importedPath string) *ImportedData
}

type ImportCacheValue struct {
	data *ImportedData

	// nil if we have only imported it via importstr
	asCode potentialValue
}

type importCacheKey struct {
	dir          string
	importedPath string
}

type importCacheMap map[importCacheKey]ImportCacheValue

type ImportCache struct {
	cache    importCacheMap
	importer Importer
}

func MakeImportCache(importer Importer) *ImportCache {
	return &ImportCache{importer: importer, cache: make(importCacheMap)}
}

func (cache *ImportCache) importData(key importCacheKey) *ImportCacheValue {
	if value, ok := cache.cache[key]; ok {
		return &value
	}
	data := cache.importer.Import(key.dir, key.importedPath)
	val := ImportCacheValue{
		data: data,
	}
	cache.cache[key] = val
	return &val
}

func (cache *ImportCache) ImportString(codeDir, importedPath string) (*valueString, error) {
	data := cache.importData(importCacheKey{codeDir, importedPath})
	if data.data.err != nil {
		return nil, data.data.err
	}
	// TODO(sbarzowski) wrap error in runtime error
	return makeValueString(data.data.content), nil
}

func (cache *ImportCache) ImportCode(codeDir, importedPath string, e *evaluator) (value, error) {
	cached := cache.importData(importCacheKey{codeDir, importedPath})
	if cached.data.err != nil {
		return nil, cached.data.err
	}
	if cached.asCode == nil {
		node, err := snippetToAST(cached.data.foundHere, cached.data.content)
		if err != nil {
			// TODO(sbarzowski) perhaps we should wrap (static) error here
			// within a RuntimeError? Because whether we get this error or not
			// actually depends on what happens in Runtime (whether import gets
			// evaluated).
			// On the other hand if the user is doing the standard reasonable thing
			// and imports unconditionally and actually uses the imports then
			// just showing the static error may be less confusing (however
			// when previous runtime error prevents a static error that's really
			// confusing)
			// Alternatively we could take advantage of imports not being
			// computed and actually do the static part statically for all
			// imports transitively.
			// The same thinking applies to external variables.
			cached.asCode = makeErrorThunk(err)
		} else {
			cached.asCode = makeThunk("import", e.i.initialEnv, node)
		}
	}
	return e.evaluate(cached.asCode)
}

// Concrete importers
// -------------------------------------

type FileImporter struct {
	// TODO(sbarzowski) fill it in
	JPaths []string
}

func tryPath(dir, importedPath string) (found bool, content []byte, foundHere string, err error) {
	var absPath string
	if path.IsAbs(importedPath) {
		absPath = importedPath
	} else {
		absPath = path.Join(dir, importedPath)
	}
	content, err = ioutil.ReadFile(absPath)
	if os.IsNotExist(err) {
		return false, nil, "", nil
	}
	return true, content, absPath, err
}

func (importer *FileImporter) Import(dir, importedPath string) *ImportedData {
	found, content, foundHere, err := tryPath(dir, importedPath)
	if err != nil {
		return &ImportedData{err: err}
	}

	for i := 0; !found && i < len(importer.JPaths); i++ {
		found, content, foundHere, err = tryPath(importer.JPaths[i], importedPath)
		if err != nil {
			return &ImportedData{err: err}
		}
	}

	return &ImportedData{content: string(content), foundHere: foundHere}
}

type MemoryImporter struct {
	data map[string]string
}

func (importer *MemoryImporter) Import(dir, importedPath string) (*ImportedData, error) {
	if content, ok := importer.data[importedPath]; ok {
		return &ImportedData{content: content, foundHere: importedPath}, nil
	}
	return nil, fmt.Errorf("Import not available %v", importedPath)
}
