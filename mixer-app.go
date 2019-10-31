// Copyright 2018 Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// nolint:lll
// Generates the mygrpcadapter adapter's resource yaml. It contains the adapter's configuration, name, supported template
// names (metric in this case), and whether it is session or no-session based.
//go:generate $GOPATH/src/istio.io/istio/bin/mixer_codegen.sh -a mixer/adapter/mygrpcadapter/config/config.proto -x "-s=false -n mygrpcadapter -t metric"

package mygrpcadapter

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
        "path/filepath"
        "flag"
	"time"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"bytes"
	"os"

	"istio.io/api/mixer/adapter/model/v1beta1"
	types "github.com/gogo/protobuf/types"

        apiv1 "k8s.io/api/core/v1"
//	appsv1 "k8s.io/api/apps/v1"
        metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
        "k8s.io/client-go/kubernetes"
        "k8s.io/client-go/tools/clientcmd"

	policy "istio.io/api/policy/v1beta1"
	"istio.io/istio/mixer/adapter/mygrpcadapter/config"
	"istio.io/istio/mixer/template/metric"
	"istio.io/istio/pkg/log"
)

type (
	// Server is basic server interface
	Server interface {
		Addr() string
		Close() error
		Run(shutdown chan error)
	}

	// MyGrpcAdapter supports metric template.
	MyGrpcAdapter struct {
		listener net.Listener
		server   *grpc.Server
	}
)

type Payload struct {
        ClusterName    string
        DeploymentName string
}



var kubeconfig *string

const ANNOTATION = "multicluster.admiralty.io/clustername"

var _ metric.HandleMetricServiceServer = &MyGrpcAdapter{}

// HandleMetric records metric entries
func (s *MyGrpcAdapter) HandleMetric(ctx context.Context, r *metric.HandleMetricRequest) (*v1beta1.ReportResult, error) {

	log.Infof("received request %v\n", *r)
	var b bytes.Buffer
	cfg := &config.Params{}

	if r.AdapterConfig != nil {
		if err := cfg.Unmarshal(r.AdapterConfig.Value); err != nil {
			log.Errorf("error unmarshalling adapter config: %v", err)
			return nil, err
		}
	}

	b.WriteString(fmt.Sprintf("HandleMetric invoked with:\n  Adapter config: %s\n  Instances: %s\n",
		cfg.String(), instances(r.Instances)))

	if cfg.FilePath == "" {
		fmt.Println(b.String())
	} else {
		_, err := os.OpenFile("out.txt", os.O_RDONLY|os.O_CREATE, 0666)
		if err != nil {
			log.Errorf("error creating file: %v", err)
		}
		f, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Errorf("error opening file for append: %v", err)
		}

		defer f.Close()

		log.Infof("writing instances to file %s", f.Name())
		if _, err = f.Write(b.Bytes()); err != nil {
			log.Errorf("error writing to file: %v", err)
		}
	}

	log.Infof("success!!")
	return &v1beta1.ReportResult{}, nil
}

func decodeDimensions(in map[string]*policy.Value) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = decodeValue(v.GetValue())
	}
	return out
}

func decodeValue(in interface{}) interface{} {
	switch t := in.(type) {
	case *policy.Value_StringValue:
		return t.StringValue
	case *policy.Value_Int64Value:
		return t.Int64Value
	case *policy.Value_DoubleValue:
		return t.DoubleValue
	case *policy.Value_DurationValue:
		return t.DurationValue.Value
	default:
		return fmt.Sprintf("%v", in)
	}
}

func homeDir() string {
        if h := os.Getenv("HOME"); h != "" {
                return h
        }
        return " "
}


func kubeConfigfile() {
        if home := homeDir(); home != "" {
                kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
        } else {
                kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
        }

}

func instances(in []*metric.InstanceMsg) string {
	var b bytes.Buffer
	for _, inst := range in {
		b.WriteString(fmt.Sprintf("'%s':\n"+
			"  {\n"+
			"		Value = %s\n"+
			"		Dimensions = %v\n"+
			"  }", inst.Name, decodeValue(inst.Value.GetValue()), decodeDimensions(inst.Dimensions)))
		threshold,_ := strconv.ParseInt(decodeValue(decodeDimensions(inst.Dimensions)["thresoldlatencymilliseconds"]).(string), 10, 64)
		latencythreshold := time.Duration(threshold)*time.Millisecond
		fmt.Println(threshold)
		fmt.Println(latencythreshold)
	        duration := decodeValue(inst.Value.GetValue()).(*types.Duration)
		reqduration,_ := types.DurationFromProto(duration)
		fmt.Println(reqduration-latencythreshold)
		if (reqduration-latencythreshold>0.0000){
			var p Payload
			p.ClusterName = "cluster2"
			p.DeploymentName = "objdetect"
		        config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		        if err != nil {
		                panic(err.Error())
		        }

		        // create the clientset
		        clientset, err := kubernetes.NewForConfig(config)
		        deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)
		        if err != nil {
		                panic(err.Error())
		        }
		        deployment, err := clientset.AppsV1().Deployments("default").Get(p.DeploymentName, metav1.GetOptions{})

		        if err != nil {
		                panic(err.Error())
		        }
		        deployment.Spec.Template.Annotations[ANNOTATION] = p.ClusterName
		        deploymentsClient.Update(deployment)
			fmt.Printf("SUCCESSSSSSSSSSSSSSSSSSS!!!!!")
		}
	}
	return b.String()
}

//func int32Ptr(i int32) *int32 { return &i }

// Addr returns the listening address of the server
func (s *MyGrpcAdapter) Addr() string {
	return s.listener.Addr().String()
}

// Run starts the server run
func (s *MyGrpcAdapter) Run(shutdown chan error) {
	shutdown <- s.server.Serve(s.listener)
}

// Close gracefully shuts down the server; used for testing
func (s *MyGrpcAdapter) Close() error {
	if s.server != nil {
		s.server.GracefulStop()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	return nil
}

func getServerTLSOption(credential, privateKey, caCertificate string) (grpc.ServerOption, error) {
	certificate, err := tls.LoadX509KeyPair(
		credential,
		privateKey,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load key cert pair")
	}
	certPool := x509.NewCertPool()
	bs, err := ioutil.ReadFile(caCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to read client ca cert: %s", err)
	}

	ok := certPool.AppendCertsFromPEM(bs)
	if !ok {
		return nil, fmt.Errorf("failed to append client certs")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    certPool,
	}
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert

	return grpc.Creds(credentials.NewTLS(tlsConfig)), nil
}

// NewMyGrpcAdapter creates a new IBP adapter that listens at provided port.
func NewMyGrpcAdapter(addr string) (Server, error) {
	if addr == "" {
		addr = "0"
	}
        flag.Parse()
	kubeConfigfile()

	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", addr))
	if err != nil {
		return nil, fmt.Errorf("unable to listen on socket: %v", err)
	}
	s := &MyGrpcAdapter{
		listener: listener,
	}
	fmt.Printf("listening on \"%v\"", s.Addr())

	credential := os.Getenv("GRPC_ADAPTER_CREDENTIAL")
	privateKey := os.Getenv("GRPC_ADAPTER_PRIVATE_KEY")
	certificate := os.Getenv("GRPC_ADAPTER_CERTIFICATE")
	if credential != "" {
		so, err := getServerTLSOption(credential, privateKey, certificate)
		if err != nil {
			return nil, err
		}
		s.server = grpc.NewServer(so)
	} else {
		s.server = grpc.NewServer()
	}
	metric.RegisterHandleMetricServiceServer(s.server, s)
	return s, nil
}
