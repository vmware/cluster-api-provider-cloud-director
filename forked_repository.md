# Fork

## Fork policies

Let's try to keep the diff to upstream as small as possible to ease rebasing, e.g. as little as possible customization
of upstream docs, use separate files instead, like the current markdown file.

Branching model:

* `giantswarm-main`: this branch represent our internal `main` branch which is used as target for internal PRs to generate a release from it. (This branch is set as `default` in github.)
* `main`: represent the upstream `main` branch

## Setup local environment

**cluster-api-provider-cloud-director**
```sh
git clone https://github.com/giantswarm/cluster-api-provider-cloud-director.git
cd cluster-api-provider-cloud-director
git remote add upstream https://github.com/vmware/cluster-api-provider-cloud-director.git
```

## Rebase `main` branch

We're regularly rebasing onto the current [upstream `main` branch](https://github.com/vmware/cluster-api-provider-cloud-director).
Rebasing is usually triggered when there are new fixes or features on upstream main we need.

Rebasing could be either done via cli

```sh
git fetch upstream
git checkout main
git pull
git rebase -i upstream/main
git push origin main
```

or via the `Sync fork` button on the github repository.
> :warning: please take care, that you are rebasing to `main` branch (and not to the default set `giantswarm-main` branch). Otherwise we might lose commits!

## Rebase `giantswarm-main` branch

Make sure you have to up-to-date `giantswarm-main` branch locally.
```sh
git checkout giantswarm-main
git pull
git rebase -i origin/main
git push --force origin giantswarm-main
```

Notes:
* after the rebase, all PRs and branches must be rebased onto the new `giantswarm-main` branch
* be careful with `git push --force` as it's very easy to lose commits with it


