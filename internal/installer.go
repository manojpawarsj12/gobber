package internal

import (
    "fmt"
    "log"
    "os"
    "path/filepath"
    "sync"
)

func checkIsInstalled(name string, version string, InstalledVersionsMutex *map[string]string, mut *sync.Mutex) bool {
    mut.Lock()
    defer mut.Unlock()
    packageData := (*InstalledVersionsMutex)[name]
    return packageData == version
}

func ancestorInstalled(name string, projectDir string, InstalledVersionsMutex *map[string]string, mut *sync.Mutex) bool {
    dir := projectDir
    for {
        mut.Lock()
        _, isInstalled := (*InstalledVersionsMutex)[name]
        mut.Unlock()
        if isInstalled {
            return true
        }
        parentDir := filepath.Dir(dir)
        if parentDir == dir {
            break
        }
        dir = parentDir
    }
    return false
}

func InstallPackageRecursive(packageData PackageData, InstalledVersionsMutex *map[string]string, projectDir string, mut *sync.Mutex) error {
    if checkIsInstalled(packageData.Name, packageData.Version, InstalledVersionsMutex, mut) {
        return nil
    }
    log.Println("installing !!!!!", packageData.Name, packageData.Version)

    if ancestorInstalled(packageData.Name, projectDir, InstalledVersionsMutex, mut) {
        return nil
    }

    // Create a directory for the package in the node_modules folder
    packageDestDir := filepath.Join(projectDir, "node_modules", packageData.Name)
    if err := os.MkdirAll(packageDestDir, 0755); err != nil {
        return fmt.Errorf("mkdir failed: %v", err)
    }

    extractionDone := make(chan error)

    go func() {
        defer close(extractionDone)

        // Extract the tarball to the package directory
        tarballUrl := packageData.Dist.Tarball
        if err := ExtractTar(&tarballUrl, &packageDestDir); err != nil {
            extractionDone <- err
            return
        }

        mut.Lock()
        (*InstalledVersionsMutex)[packageData.Name] = packageData.Version
        mut.Unlock()
        extractionDone <- nil
    }()

    deps := packageData.Dependencies
    depErrChan := make(chan error, len(deps))

    var wg sync.WaitGroup
    for name, version := range deps {
        wg.Add(1)
        go func(name, version string) {
            defer wg.Done()
            comparator, err := v.parseSemanticVersion(version)
            if err != nil {
                depErrChan <- fmt.Errorf("error parsing semantic version: %v", err)
                return
            }
            packageDetails := PackageDetails{Name: name, Comparator: comparator}
            depPackageData, err := GetPackageData(&packageDetails)
            if err != nil {
                depErrChan <- err
                return
            }
            // Install dependencies recursively inside the packageDestDir
            if err := InstallPackageRecursive(depPackageData, InstalledVersionsMutex, packageDestDir, mut); err != nil {
                depErrChan <- err
                return
            }
            depErrChan <- nil
        }(name, version)
    }

    go func() {
        wg.Wait()
        close(depErrChan)
    }()

    for err := range depErrChan {
        if err != nil {
            return fmt.Errorf("error installing dependencies: %v", err)
        }
    }

    // Wait for tarball extraction to complete
    if err := <-extractionDone; err != nil {
        return fmt.Errorf("error extracting tarball: %v", err)
    }
    return nil
}

func Execute(packageDetails *PackageDetails, mut *sync.Mutex, InstalledVersionsMutex *map[string]string, projectDir string) error {
    done := make(chan error)

    go func() {
        packageData, err := GetPackageData(packageDetails)
        if err != nil {
            done <- err
            return
        }
        if err := InstallPackageRecursive(packageData, InstalledVersionsMutex, projectDir, mut); err != nil {
            done <- err
            return
        }
        done <- nil
    }()

    if err := <-done; err != nil {
        return fmt.Errorf("error executing package installation: %v", err)
    }
    return nil
}