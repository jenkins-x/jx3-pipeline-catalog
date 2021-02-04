# jx3-pipeline-catalog

The default pipeline catalog for Jenkins X 3.x

Jenkins X 3.x comes with its own default pipeline catalog for different languages, tools and frameworks. This catalog contains reusable steps, Tasks, Pipelines and Packs you can use on any project.

For more information check out the [Jenkins X 3.x support for Tekton Catalog](https://jenkins-x.io/v3/develop/pipeline-catalog/)


## Contents

* [tasks](tasks) a reusable folder of tasks and associated triggers
* [packs](packs) contains the language and/or framework specific packs containing tekton pipelines and associated files used by the pipelines such as `Dockerfile` or helm charts.
  * e.g. the [javascript](packs/javascript) pack has the Jenkins X pipelines at [packs/javascript/.lighthouse/jenkins-x](packs/javascript/.lighthouse/jenkins-x)
* [helm](helm) contains reusable helm charts that are imported into the various folders in [packs](packs) such as [packs/javascript/charts](packs/javascript/charts) to share charts across the different programming languages


## Custom Pipeline Catalogs
([Blog article](https://jenkins-x.io/blog/2020/11/11/accelerate-tekton/#custom-pipeline-catalogs))

To use your own custom pipeline catalog, you can [fork this catalog](https://github.com/jenkins-x/jx3-pipeline-catalog/fork) to make changes for your team or share between teams in your company. You can make as many catalogs as you like and put whichever catalogs you want in the extensions/pipeline-catalogs.yaml file of your cluster git repository of your Jenkins X 3.x install, like for example:
```
# Source: <boot-repo>/extensions/pipeline-catalog.yaml
apiVersion: project.jenkins-x.io/v1alpha1
kind: PipelineCatalog
spec:
  repositories:
  - label: Your Pipeline Catalog
    gitUrl: https://github.com/<your-org>/jx3-pipeline-catalog
    gitRef: master
```


For more detail there's [the configuration reference here](https://github.com/jenkins-x/jx-project/blob/master/docs/config.md#project.jenkins-x.io/v1alpha1.PipelineCatalog).

Then when developers create a new quickstart or import a repository developers will be asked to pick the catalog they want from your list if there is more than one, or the configured catalog is silently used.

This gives you complete freedom to configure things at a global, team or repository level while also making it easy to share changes across projects, teams and companies.
