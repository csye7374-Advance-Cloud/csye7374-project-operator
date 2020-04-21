package appservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	csye7374v1alpha1 "github.com/yogitapatil/csye7374-termproject-operator/pkg/apis/csye7374/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

var log = logf.Log.WithName("controller_appservice")

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new AppService Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAppService{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("appservice-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource AppService
	err = c.Watch(&source.Kind{Type: &csye7374v1alpha1.AppService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Pods and requeue the owner AppService
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &csye7374v1alpha1.AppService{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileAppService implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileAppService{}

// ReconcileAppService reconciles a AppService object
type ReconcileAppService struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a AppService object and makes changes based on the state read
// and what is in the AppService.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAppService) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling AppService")

	// Fetch the AppService instance
	instance := &csye7374v1alpha1.AppService{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// Get AWS credendtails and S3 bucket name from secret
	awsAccessKey, awsSecretKey, s3BucketName, err := r.getDataFromSecret("aws-secret", instance.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	} 

	reqLogger.Info("Creating new AWS session")

	sess, err := createAwsSession(awsAccessKey, awsSecretKey, "us-east-1")
	if(err != nil){
		return reconcile.Result{}, err
	}

	reqLogger.Info("Creating folder with name" + instance.Spec.UserName + "inside S3 bucket")	
	// Create folder inside S3 bucket
	err = createFolderS3Bucket(instance, sess, s3BucketName)
	if err != nil {
		return reconcile.Result{}, err
	} 

	// Create IAM service client
	svc := iam.New(sess)

	// Create IAM user
	reqLogger.Info("Creating IAM user")	
	createdIamUser,err := createIamUser(svc, instance)
	if err != nil {
		return reconcile.Result{}, err
	} 

	// TODO: Created policy using the above created user and Attach policy to the user

	//Create access and secret keys 
	accessKeyId, err := getAccessKey(svc, aws.StringValue(createdIamUser.UserName))

	// check if secret exists
	foundUserSecret := &corev1.Secret{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: instance.Spec.UserSecret.Name, Namespace: instance.Namespace}, foundUserSecret)
	if err != nil{
		// Secret does not exists
		if errors.IsNotFound(err){
			// check if access key already exists in AWS
			if len(accessKeyId)!=0{
				// delete the old access key
				err = deleteAccessKey(svc, accessKeyId, aws.StringValue(createdIamUser.UserName))
				if err != nil{
					return reconcile.Result{}, err
				}
			}
			
			// create new access keys
			accessKey, err := createAccessKey(svc , aws.StringValue(createdIamUser.UserName))
			if err != nil{
				return reconcile.Result{}, err
			}

			// create secret
			secret := newSecret(instance, accessKey)

			// Set AppService instance as the owner and controller
			if err = controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
				return reconcile.Result{}, err
			}

			reqLogger.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
			// Create new secret
			err = r.client.Create(context.TODO(), secret)
			if err != nil {
				return reconcile.Result{}, err
			}
		
			reqLogger.Info("Secret created Successfully")
			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err 
	}
	
	// Secret exists
	var shouldRecreateAccessKeyAndSecret bool
	// Check if access key in AWS matches the access key from Secret. 
	// If not; delete the old access key and recreate secret with new access key. 
	// If matches, don't do anything
	if len(accessKeyId)!=0{
		if accessKeyId != string(foundUserSecret.Data["aws_accesskey"]){
			// delete the old access key from AWS and create secret
			err = deleteAccessKey(svc, accessKeyId, aws.StringValue(createdIamUser.UserName))
			if err != nil{
				return reconcile.Result{}, err
			}	
			shouldRecreateAccessKeyAndSecret = true
		}
		reqLogger.Info("Matching access key found")
	}else{
		shouldRecreateAccessKeyAndSecret = true
	}

	if shouldRecreateAccessKeyAndSecret{
		// create new access keys
		accessKey, err := createAccessKey(svc , aws.StringValue(createdIamUser.UserName))
		if err != nil{
				return reconcile.Result{}, err
		}

		// Delete existing secret
		err = r.client.Delete(context.TODO(), foundUserSecret)
		if err != nil{
			return reconcile.Result{}, err
		}
		// create secret and return
		secret := newSecret(instance, accessKey)

		// Set AppService instance as the owner and controller
		if err := controllerutil.SetControllerReference(instance, secret, r.scheme); err != nil {
			return reconcile.Result{}, err
		}

		reqLogger.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		// Create new secret
		err = r.client.Create(context.TODO(), secret)
		if err != nil {
			return reconcile.Result{}, err
		}
		
		reqLogger.Info("Secret created Successfully")
		return reconcile.Result{}, nil
	}

	// Secret already exists - don't requeue
	reqLogger.Info("Skip reconcile: Secret already exists", "Secret.Namespace", foundUserSecret.Namespace, "Secret.Name", foundUserSecret.Name)
	return reconcile.Result{RequeueAfter: time.Second * 10}, nil
}

func createAwsSession(accessKeyID string, secretAccessKey string, region string) (*session.Session, error){
	s, err := session.NewSession(&aws.Config{
        Region: aws.String(region),
        Credentials: credentials.NewStaticCredentials(
            accessKeyID,
            secretAccessKey,
            ""),
	})

	if err != nil {
		return nil, err
	}

	return s, nil
}

func (r *ReconcileAppService) getDataFromSecret(secretName , namespace string) (string, string, string, error){
	secret := &corev1.Secret{}
	err := r.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		},
		secret)

	if err != nil {
		fmt.Println("Cannot retrieve secret", secretName)
		return "","", "", err
	}

	accessKeyID, ok := secret.Data["aws_key"]
	if !ok {
		return "","", "", fmt.Errorf("Secret %v did not contain key %v",
			secretName, "aws_key")
	}

	secretAccessKey, ok := secret.Data["secret_key"]
	if !ok {
		return "","", "", fmt.Errorf("Secret %v did not contain key %v",
			secretName, "secret_key")
	}

	s3BucketName, ok := secret.Data["s3_bucket"]
	if !ok {
		return "","", "", fmt.Errorf("Secret %v did not contain key %v",
			secretName, "s3_bucket")
	}
	//fmt.Println("bucketname: " + strings.Trim(string(s3BucketName),"\n"))

	return strings.Trim(string(accessKeyID), "\n"),strings.Trim(string(secretAccessKey),"\n"), strings.Trim(string(s3BucketName),"\n"), nil
}

func createFolderS3Bucket(cr *csye7374v1alpha1.AppService, s *session.Session, s3BucketName string) (error) {

	folderName := cr.Spec.UserName + "/"

	// create new folder
	_,err := s3.New(s).PutObject(&s3.PutObjectInput{
        Bucket:               aws.String(s3BucketName),
        Key:                  aws.String(folderName),
    })

	if err != nil {
		return err
	}

	return nil
}

// Create IAM user
func createIamUser(svc *iam.IAM, cr *csye7374v1alpha1.AppService) (*iam.User, error){
	u, err := svc.GetUser(&iam.GetUserInput{
		UserName: aws.String(cr.Spec.UserName),
	})

	if awserr, ok := err.(awserr.Error); ok && awserr.Code() == iam.ErrCodeNoSuchEntityException {
		result, err := svc.CreateUser(&iam.CreateUserInput{
			UserName: aws.String(cr.Spec.UserName),
		})

		if err != nil {
			fmt.Println("Error while creating user", err)
			return nil,err
		}

		fmt.Println("User created", result.User)
		return result.User,nil
	} else {
		fmt.Println("User already exists")
		return u.User, nil
	}
}

// Create access keys
func createAccessKey(svc *iam.IAM, userName string) (*iam.AccessKey, error){
	
	result, err := svc.CreateAccessKey(&iam.CreateAccessKeyInput{
        UserName: aws.String(userName),
    })

    if err != nil {
        fmt.Println("Error while creating access keys", err)
        return nil, err
    }

	fmt.Println("Success- Access key created")
	return result.AccessKey, nil
}

func getAccessKey(svc *iam.IAM, userName string) (string, error){
	keys, err := svc.ListAccessKeys(&iam.ListAccessKeysInput{
        UserName: aws.String(userName),
    })

    if err != nil {
        fmt.Println("Could not get access key", err)
        return "", err
	}
	
	//fmt.Println("Success- Accesskey list", keys)

	for _,v := range keys.AccessKeyMetadata{
		//fmt.Println("AccessKeyId", string(*v.AccessKeyId))
		return string(*v.AccessKeyId), nil
		}

	fmt.Println("No access key found")
	return "", nil
}

func deleteAccessKey(svc *iam.IAM, accessKeyId string, userName string) error{

	_, err := svc.DeleteAccessKey(&iam.DeleteAccessKeyInput{
		AccessKeyId: aws.String(accessKeyId),
		UserName:    aws.String(userName),
	})
	if err != nil {
		fmt.Println("Deleted old access key successful")
		return err
	}
	return nil
}

// Create new secret
func newSecret(cr *csye7374v1alpha1.AppService, accessKey *iam.AccessKey) *corev1.Secret {
	labels := map[string]string{
		"user-secret": cr.Name,
	}
	return &corev1.Secret{
		Type: corev1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Spec.UserSecret.Name,
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"aws_accesskey":     []byte(aws.StringValue(accessKey.AccessKeyId)),
			"aws_secretKey": 	 []byte(aws.StringValue(accessKey.SecretAccessKey)),
		},
	}
}

// newPodForCR returns a busybox pod with the same name/namespace as the cr
func newPodForCR(cr *csye7374v1alpha1.AppService) *corev1.Pod {
	labels := map[string]string{
		"app": cr.Name,
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cr.Name + "-pod",
			Namespace: cr.Namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "busybox",
					Command: []string{"sleep", "3600"},
				},
			},
		},
	}
}