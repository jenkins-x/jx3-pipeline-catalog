#!/bin/bash

exec >> logfile.txt 2>&1

CLUSTER_REPO=demo-jx3-gke-gsm
SCRIPTNAME=`basename "$0"`
SCRIPTDIR=$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )

while getopts u:t:o:a:h flag
do
    case "${flag}" in
        u) GITUSER=${OPTARG};;
        t) GITTOKEN=${OPTARG};;
        o) ORGANIZATION=${OPTARG};;
        a) APPNAME=${OPTARG};;
        h) echo "Usage: $SCRIPTNAME -u <git user> -t <git token> -o <organization> -a <app to delete>";echo "       -h show this help";exit 0;;
    esac
done

# only ask if in interactive mode
if [[ -z ${GITUSER} ]];then
  echo -n "Gituser ? "
  read -r GITUSER
fi

if [[ -z ${GITTOKEN} ]];then
  echo -n "Git token ? "
  read -r GITTOKEN
fi

if [[ -z ${APPNAME} ]];then
  echo -n "Application to delete ? "
  read -r APPNAME
fi

if [[ -z ${ORGANIZATION} ]];then
  echo -n "Git organization ? "
  read -r ORGANIZATION
fi

#echo "Git user: $GITUSER";
#echo "Git token: $GITTOKEN";
#echo "Git organization: $ORGANIZATION";
#echo "App name: $APPNAME";

# Debug
set -x

echo "`date`: Deleting app $APPNAME..."

echo "Cloning or pulling cluster repository..."

git clone https://oauth2:$GITTOKEN@github.com/${ORGANIZATION}/${CLUSTER_REPO}.git || (echo "Repo already exists, pulling..." && git -C ${CLUSTER_REPO}/ pull origin)

cd ${CLUSTER_REPO}

echo "Deleting app from staging"
# Delete chart entry in the helmfiles/jx-staging/helmfile.yaml
awk -i inplace -v pattern="- chart: dev\/$APPNAME" 'BEGIN{ print_flag=1 } 
{
    if( $0 ~ pattern ) 
    {
       print_flag=0;
       next
    } 
    if( $0 ~ /^[a-zA-Z0-9\d-]/ ) 
    {
        print_flag=1;   
    } 
    if ( print_flag == 1 ) 
        print $0

} ' helmfiles/jx-staging/helmfile.yaml

# echo "Deleting app from production"
# Delete chart entry in the helmfiles/jx-production/helmfile.yaml
# TODO

echo "Deleting app in config-root staging..."
# Delete application folder in jx-staging config-root
rm -rf config-root/namespaces/jx-staging/$APPNAME

# echo "Deleting app in config-root production..."
# Delete application folder in jx-production config-root
# TODO
# rm -rf config-root/namespaces/jx-production/$APPNAME

echo "Deleting repositories entries in cluster repo..."
# Delete yaml file in source-repositories
rm config-root/namespaces/jx/source-repositories/$ORGANIZATION-$APPNAME.yaml
# Delete entries from .jx/gitops/source-config.yaml
sed -i "/$APPNAME\$/d" .jx/gitops/source-config.yaml

echo "Pushing to GitOps repository"
git add .
git commit -m "Delete app $APPNAME"
git push

# echo "Deleting repositories resources in jx namespace..."
# Delete sr resources in jx namespace
kubectl -n jx delete sr $ORGANIZATION-$APPNAME

echo "Deleting  resources in staging namespace created for the database dependency..."
# Delete resources in staging created for the database dependency
kubectl -n jx-staging delete externalsecret $APPNAME-${APPNAME}db
kubectl -n jx-staging delete secret $APPNAME-${APPNAME}db
kubectl -n jx-staging delete secret $(kubectl -n jx-staging get secret | grep $APPNAME-$APPNAME-token-* | awk NF=1)
kubectl -n jx-staging delete serviceaccount $APPNAME-$APPNAME
kubectl patch pvc data-$APPNAME-${APPNAME}db-0 -p '{"metadata":{"finalizers":null}}'
kubectl -n jx-staging delete pvc data-$APPNAME-${APPNAME}db-0
