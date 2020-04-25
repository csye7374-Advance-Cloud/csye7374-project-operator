# CSYE 7374 - Spring 2020  Project

## Team Information

| Name | NEU ID | Email Address |
| --- | --- | --- |
|Jagmandeep Kaur | 001426439|kaur.j@husky.neu.edu |  | | |
|Mayank Barua| 001475187| barua.m@husky.neu.edu|
|Yogita Patil| 001435442|patil.yo@husky.neu.edu |
|Prajesh Jain| 001409343| Jain.pra@husky.neu.edu|

## To Deploy
```
kubectl create -f deploy/service_account.yaml
kubectl create -f deploy/role.yaml
kubectl create -f deploy/role_binding.yaml
kubectl create -f deploy/crds/csye7374.app.com_appservices_crd.yaml
kubectl create -f deploy/secret.yaml
kubectl create -f deploy/docker-secret.yaml
```

## To Build Image
```
operator-sdk build registryName/repoName:tag
```
## Push
```
docker push imagename
```

## To Update 
```
deploy/operator.yaml with the new image
kubectl create -f deploy/operator.yaml
kubectl create -f deploy/crds/csye7374.app.com_v1alpha1_appservice_cr.yaml
```

## To Delete Resources
```
kubectl delete -f deploy/crds/csye7374.app.com_v1alpha1_appservice_cr.yaml
kubectl delete -f deploy/operator.yaml
kubectl delete -f deploy/role.yaml
kubectl delete -f deploy/role_binding.yaml
kubectl delete -f deploy/service_account.yaml
kubectl delete -f deploy/crds/app.example.com_appservices_crd.yaml
```
