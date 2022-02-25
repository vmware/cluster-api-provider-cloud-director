#!/bin/bash
catch() {
  retval=$?
  error_message="$(date) $(caller): $BASH_COMMAND"
  echo "$error_message" &> error.log
}
trap 'catch $? $LINENO' ERR
set -ex

# Reads go.sum and returns its contents with the following syntax:
# <module>:<version>
function getGoModules {
    grep -v 'go.mod' go.sum | awk '{print "\""$1 ":" $2 "\","}'
}

# Returns the given amount of spaces, intended for JSON identation
function indent {
    for _ in $(seq 1 $1)
    do
        printf ' '
    done
}

# Gets the "components" elements for the Provenance JSON
function getComponents {
    rawGoModules=$(getGoModules)
    for module in $rawGoModules
    do
        indent 16
        echo "{"
        indent 20
        echo "\"name\": $module"
        indent 20
        echo "\"incorporated\": true"
        indent 16
        echo "},"
    done
}

# ---------------
# Script init
# ---------------

if [ $# != 2 ]
then
    echo "Usage: ./provenance.sh <PROJECT NAME> <RELEASE_TAG>"
    echo ""
    echo "PROJECT NAME: Either 'terraform-provider-vcd' or 'go-vcloud-director' or 'cluster-api-provider-cloud-director'"
    echo "RELEASE_TAG: Release tag for which the provenance data should be genereated"
    exit 1
fi

project="$1"
release_tag="$2"

if [[ "$project" != 'terraform-provider-vcd' ]] && [[ "$project" != 'go-vcloud-director' ]] && [[ "$project" != 'cluster-api-provider-cloud-director' ]]
then
    echo "PROJECT NAME must be either 'terraform-provider-vcd' or 'go-vcloud-director'"
    exit 1
fi

tmpDir='tmp'
rm -rf $tmpDir
git clone https://github.com/vmware/$project.git $tmpDir
pushd $tmpDir
git checkout $release_tag -b $release_tag

head=$(git log -1 --pretty=format:%H)
version=$(git describe --tags --abbrev=0)
identifier=$(git describe --tags)
components=$(getComponents)

provenanceJsonTemplate="
{
    \"id\": \"http://vmware.com/schemas/software_provenance-0.2.5.json\",
    \"root\": [\"$project\"],
    \"all_components\": {
        \"$project-$identifier\": {
            \"typename\": \"comp.build\",
            \"name\": \"$project\",
            \"version\": \"$version\",
            \"source_repositories\": [
                {
                    \"content\": \"source\",
                    \"branch\": \"main\",
                    \"host\": \"github.com\",
                    \"path\": \"vmware/$project\",
                    \"ref\": \"$head\",
                    \"protocol\": \"git\"
                }
            ],
            \"target_repositories\": [
                {
                    \"content\": \"binary\",
                    \"protocol\": \"https\",
                    \"host\": \"github.com\",
                    \"path\": [
                        \"vmware/$project/releases/tag/$version\"
                    ]
                }
            ],
            \"components\": [
                ${components%,}
            ],
            \"actions\": {
                \"edit-changelog\": {
                    \"typename\": \"action\"
                },
                \"create-zipped-binaries\": {
                    \"typename\": \"action\"
                },
                \"create-github-tag\": {
                    \"typename\": \"action\"
                },
                \"prepare-next-version\": {
                    \"typename\": \"action\"
                }
            }
        }
    }
}"

cd ..
rm -rf $tmpDir
echo "$provenanceJsonTemplate" > provenance-$project-$version.json
