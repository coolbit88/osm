package smi

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	smiAccess "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/access/v1alpha2"
	smiSpecs "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/specs/v1alpha3"
	smiSplit "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	testTrafficTargetClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/access/clientset/versioned/fake"
	testTrafficSpecClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/specs/clientset/versioned/fake"
	testTrafficSplitClient "github.com/servicemeshinterface/smi-sdk-go/pkg/gen/client/split/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclient "k8s.io/client-go/kubernetes/fake"

	osmPolicy "github.com/openservicemesh/osm/experimental/pkg/apis/policy/v1alpha1"
	osmPolicyClient "github.com/openservicemesh/osm/experimental/pkg/client/clientset/versioned/fake"
	"github.com/openservicemesh/osm/pkg/constants"
	"github.com/openservicemesh/osm/pkg/featureflags"
	"github.com/openservicemesh/osm/pkg/namespace"
	"github.com/openservicemesh/osm/pkg/service"
	"github.com/openservicemesh/osm/pkg/tests"
)

const (
	testNamespaceName = "test"
)

type fakeKubeClientSet struct {
	kubeClient                *testclient.Clientset
	smiTrafficSplitClientSet  *testTrafficSplitClient.Clientset
	smiTrafficSpecClientSet   *testTrafficSpecClient.Clientset
	smiTrafficTargetClientSet *testTrafficTargetClient.Clientset
	osmPolicyClientSet        *osmPolicyClient.Clientset
}

func bootstrapClient() (MeshSpec, *fakeKubeClientSet, error) {
	osmNamespace := "osm-system"
	meshName := "osm"
	stop := make(chan struct{})
	kubeClient := testclient.NewSimpleClientset()
	smiTrafficSplitClientSet := testTrafficSplitClient.NewSimpleClientset()
	smiTrafficSpecClientSet := testTrafficSpecClient.NewSimpleClientset()
	smiTrafficTargetClientSet := testTrafficTargetClient.NewSimpleClientset()
	osmPolicyClientSet := osmPolicyClient.NewSimpleClientset()
	namespaceController := namespace.NewNamespaceController(kubeClient, meshName, stop)

	fakeClientSet := &fakeKubeClientSet{
		kubeClient:                kubeClient,
		smiTrafficSplitClientSet:  smiTrafficSplitClientSet,
		smiTrafficSpecClientSet:   smiTrafficSpecClientSet,
		smiTrafficTargetClientSet: smiTrafficTargetClientSet,
		osmPolicyClientSet:        osmPolicyClientSet,
	}

	// Create a test namespace that is monitored
	testNamespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNamespaceName,
			Labels: map[string]string{constants.OSMKubeResourceMonitorAnnotation: meshName}, // Label selectors don't work with fake clients, only here to signify its importance
		},
	}
	if _, err := kubeClient.CoreV1().Namespaces().Create(context.TODO(), &testNamespace, metav1.CreateOptions{}); err != nil {
		log.Fatal().Err(err).Msgf("Error creating Namespace %v", testNamespace)
	}
	<-namespaceController.GetAnnouncementsChannel()

	meshSpec, err := newSMIClient(
		kubeClient,
		smiTrafficSplitClientSet,
		smiTrafficSpecClientSet,
		smiTrafficTargetClientSet,
		osmPolicyClientSet,
		osmNamespace,
		namespaceController,
		kubernetesClientName,
		stop,
	)

	return meshSpec, fakeClientSet, err
}

var _ = Describe("When listing TrafficSplit", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)
	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return a list of traffic split resources", func() {
		split := &smiSplit.TrafficSplit{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ListTrafficSplits",
				Namespace: testNamespaceName,
			},
			Spec: smiSplit.TrafficSplitSpec{
				Service: tests.BookstoreApexServiceName,
				Backends: []smiSplit.TrafficSplitBackend{
					{
						Service: tests.BookstoreServiceName,
						Weight:  tests.Weight,
					},
				},
			},
		}

		_, err := fakeClientSet.smiTrafficSplitClientSet.SplitV1alpha2().TrafficSplits(testNamespaceName).Create(context.TODO(), split, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		splits := meshSpec.ListTrafficSplits()
		Expect(len(splits)).To(Equal(1))
		Expect(split).To(Equal(splits[0]))

		err = fakeClientSet.smiTrafficSplitClientSet.SplitV1alpha2().TrafficSplits(testNamespaceName).Delete(context.TODO(), split.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})
})

var _ = Describe("When listing TrafficSplit services", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)
	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return a list of weighted services corresponding to the traffic split backends", func() {
		split := &smiSplit.TrafficSplit{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ListTrafficSplitServices",
				Namespace: testNamespaceName,
			},
			Spec: smiSplit.TrafficSplitSpec{
				Service: tests.BookstoreApexServiceName,
				Backends: []smiSplit.TrafficSplitBackend{
					{
						Service: "bookstore-v1",
						Weight:  tests.Weight,
					},
					{
						Service: "bookstore-v2",
						Weight:  tests.Weight,
					},
				},
			},
		}

		_, err := fakeClientSet.smiTrafficSplitClientSet.SplitV1alpha2().TrafficSplits(testNamespaceName).Create(context.TODO(), split, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		weightedServices := meshSpec.ListTrafficSplitServices()
		Expect(len(weightedServices)).To(Equal(len(split.Spec.Backends)))
		for i, backend := range split.Spec.Backends {
			Expect(weightedServices[i].Service).To(Equal(service.MeshService{Namespace: split.Namespace, Name: backend.Service}))
			Expect(weightedServices[i].Weight).To(Equal(backend.Weight))
			Expect(weightedServices[i].RootService).To(Equal(split.Spec.Service))
		}

		err = fakeClientSet.smiTrafficSplitClientSet.SplitV1alpha2().TrafficSplits(testNamespaceName).Delete(context.TODO(), split.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})
})

var _ = Describe("When listing ServiceAccounts", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)
	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return a list of service accounts specified in TrafficTarget resources", func() {
		trafficTarget := &smiAccess.TrafficTarget{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "access.smi-spec.io/v1alpha2",
				Kind:       "TrafficTarget",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ListServiceAccounts",
				Namespace: testNamespaceName,
			},
			Spec: smiAccess.TrafficTargetSpec{
				Destination: smiAccess.IdentityBindingSubject{
					Kind:      "Name",
					Name:      tests.BookstoreServiceAccountName,
					Namespace: testNamespaceName,
				},
				Sources: []smiAccess.IdentityBindingSubject{{
					Kind:      "Name",
					Name:      tests.BookbuyerServiceAccountName,
					Namespace: testNamespaceName,
				}},
				Rules: []smiAccess.TrafficTargetRule{{
					Kind:    "HTTPRouteGroup",
					Name:    tests.RouteGroupName,
					Matches: []string{tests.BuyBooksMatchName},
				}},
			},
		}

		_, err := fakeClientSet.smiTrafficTargetClientSet.AccessV1alpha2().TrafficTargets(testNamespaceName).Create(context.TODO(), trafficTarget, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		svcAccounts := meshSpec.ListServiceAccounts()

		numExpectedSvcAccounts := len(trafficTarget.Spec.Sources) + 1 // 1 for the destination ServiceAccount
		Expect(len(svcAccounts)).To(Equal(numExpectedSvcAccounts))

		err = fakeClientSet.smiTrafficTargetClientSet.AccessV1alpha2().TrafficTargets(testNamespaceName).Delete(context.TODO(), trafficTarget.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})
})

var _ = Describe("When listing TrafficTargets", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)
	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("Returns a list of TrafficTarget resources", func() {
		trafficTarget := &smiAccess.TrafficTarget{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "access.smi-spec.io/v1alpha2",
				Kind:       "TrafficTarget",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-ListTrafficTargets",
				Namespace: testNamespaceName,
			},
			Spec: smiAccess.TrafficTargetSpec{
				Destination: smiAccess.IdentityBindingSubject{
					Kind:      "Name",
					Name:      tests.BookstoreServiceAccountName,
					Namespace: testNamespaceName,
				},
				Sources: []smiAccess.IdentityBindingSubject{{
					Kind:      "Name",
					Name:      tests.BookbuyerServiceAccountName,
					Namespace: testNamespaceName,
				}},
				Rules: []smiAccess.TrafficTargetRule{{
					Kind:    "HTTPRouteGroup",
					Name:    tests.RouteGroupName,
					Matches: []string{tests.BuyBooksMatchName},
				}},
			},
		}

		_, err := fakeClientSet.smiTrafficTargetClientSet.AccessV1alpha2().TrafficTargets(testNamespaceName).Create(context.TODO(), trafficTarget, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		targets := meshSpec.ListTrafficTargets()
		Expect(len(targets)).To(Equal(1))

		err = fakeClientSet.smiTrafficTargetClientSet.AccessV1alpha2().TrafficTargets(testNamespaceName).Delete(context.TODO(), trafficTarget.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})
})

var _ = Describe("When fetching a Service corresponding to a Meshservice", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)
	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return a Service resource corresponding to the given service", func() {
		meshSvc := service.MeshService{
			Namespace: testNamespaceName,
			Name:      "test-GetService",
		}
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      meshSvc.Name,
				Namespace: meshSvc.Namespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:     "servicePort",
					Protocol: corev1.ProtocolTCP,
					Port:     tests.ServicePort,
				}},
			},
		}

		_, err := fakeClientSet.kubeClient.CoreV1().Services(testNamespaceName).Create(context.TODO(), svc, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		svcIncache := meshSpec.GetService(meshSvc)
		Expect(svcIncache).To(Equal(svc))

		err = fakeClientSet.kubeClient.CoreV1().Services(testNamespaceName).Delete(context.TODO(), svc.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})

	It("should return nil when the given MeshService is not found", func() {
		meshSvc := service.MeshService{
			Namespace: testNamespaceName,
			Name:      "test-GetService",
		}

		svcIncache := meshSpec.GetService(meshSvc)
		Expect(svcIncache).To(BeNil())
	})
})

var _ = Describe("When listing Services", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)
	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return an empty list when no services are found", func() {
		services := meshSpec.ListServices()
		Expect(len(services)).To(Equal(0))
	})

	It("should return a list of Services", func() {
		svc1 := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-1",
				Namespace: testNamespaceName,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:     "servicePort",
					Protocol: corev1.ProtocolTCP,
					Port:     tests.ServicePort,
				}},
			},
		}
		svc2 := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-2",
				Namespace: testNamespaceName,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Name:     "servicePort",
					Protocol: corev1.ProtocolTCP,
					Port:     tests.ServicePort,
				}},
			},
		}

		_, err := fakeClientSet.kubeClient.CoreV1().Services(testNamespaceName).Create(context.TODO(), svc1, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
		_, err = fakeClientSet.kubeClient.CoreV1().Services(testNamespaceName).Create(context.TODO(), svc2, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		services := meshSpec.ListServices()
		Expect(len(services)).To(Equal(2))

		expectedServices := []string{"test-1", "test-2"}
		Expect(len(services)).To(Equal(len(expectedServices)))
		Expect(services[0].Name).To(BeElementOf(expectedServices))
		Expect(services[1].Name).To(BeElementOf(expectedServices))

		err = fakeClientSet.kubeClient.CoreV1().Services(testNamespaceName).Delete(context.TODO(), svc1.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
		err = fakeClientSet.kubeClient.CoreV1().Services(testNamespaceName).Delete(context.TODO(), svc2.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})
})

var _ = Describe("When listing ListHTTPTrafficSpecs", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)
	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("Returns an empty list when no HTTPRouteGroup are found", func() {
		services := meshSpec.ListHTTPTrafficSpecs()
		Expect(len(services)).To(Equal(0))
	})

	It("should return a list of ListHTTPTrafficSpecs resources", func() {
		routeSpec := &smiSpecs.HTTPRouteGroup{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "specs.smi-spec.io/v1alpha3",
				Kind:       "HTTPRouteGroup",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespaceName,
				Name:      "test-ListHTTPTrafficSpecs",
			},
			Spec: smiSpecs.HTTPRouteGroupSpec{
				Matches: []smiSpecs.HTTPMatch{
					{
						Name:      tests.BuyBooksMatchName,
						PathRegex: tests.BookstoreBuyPath,
						Methods:   []string{"GET"},
						Headers: map[string]string{
							"user-agent": tests.HTTPUserAgent,
						},
					},
					{
						Name:      tests.SellBooksMatchName,
						PathRegex: tests.BookstoreSellPath,
						Methods:   []string{"GET"},
					},
					{
						Name: tests.WildcardWithHeadersMatchName,
						Headers: map[string]string{
							"user-agent": tests.HTTPUserAgent,
						},
					},
				},
			},
		}

		_, err := fakeClientSet.smiTrafficSpecClientSet.SpecsV1alpha3().HTTPRouteGroups(testNamespaceName).Create(context.TODO(), routeSpec, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		httpRoutes := meshSpec.ListHTTPTrafficSpecs()
		Expect(len(httpRoutes)).To(Equal(1))
		Expect(httpRoutes[0].Name).To(Equal(routeSpec.Name))

		err = fakeClientSet.smiTrafficSpecClientSet.SpecsV1alpha3().HTTPRouteGroups(testNamespaceName).Delete(context.TODO(), routeSpec.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})
})

var _ = Describe("When fetching BackpressurePolicy for the given MeshService", func() {
	var (
		meshSpec      MeshSpec
		fakeClientSet *fakeKubeClientSet
		err           error
	)

	It("should returns nil when a Backpressure feature is disabled", func() {
		meshSvc := service.MeshService{
			Namespace: testNamespaceName,
			Name:      "test-GetBackpressurePolicy",
		}
		backpressure := meshSpec.GetBackpressurePolicy(meshSvc)
		Expect(backpressure).To(BeNil())
	})

	// Initialize feature for unit testing
	optional := featureflags.OptionalFeatures{
		Backpressure: true,
	}
	featureflags.Initialize(optional)

	BeforeEach(func() {
		meshSpec, fakeClientSet, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should returns nil when a Backpressure policy does not exist for the given service", func() {
		meshSvc := service.MeshService{
			Namespace: testNamespaceName,
			Name:      "test-GetBackpressurePolicy",
		}
		backpressure := meshSpec.GetBackpressurePolicy(meshSvc)
		Expect(backpressure).To(BeNil())
	})

	It("should return the Backpresure policy for the given service", func() {
		meshSvc := service.MeshService{
			Namespace: testNamespaceName,
			Name:      "test-GetBackpressurePolicy",
		}
		backpressurePolicy := &osmPolicy.Backpressure{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "policy.openservicemesh.io/v1alpha1",
				Kind:       "Backpressure",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespaceName,
				Name:      "test-GetBackpressurePolicy",
				Labels:    map[string]string{"app": meshSvc.Name},
			},
			Spec: osmPolicy.BackpressureSpec{
				MaxConnections: 123,
			},
		}

		_, err := fakeClientSet.osmPolicyClientSet.PolicyV1alpha1().Backpressures(testNamespaceName).Create(context.TODO(), backpressurePolicy, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		backpressurePolicyInCache := meshSpec.GetBackpressurePolicy(meshSvc)
		Expect(backpressurePolicyInCache).ToNot(BeNil())
		Expect(backpressurePolicyInCache.Name).To(Equal(backpressurePolicy.Name))

		err = fakeClientSet.osmPolicyClientSet.PolicyV1alpha1().Backpressures(testNamespaceName).Delete(context.TODO(), backpressurePolicy.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})

	It("should return nil when the app label is missing for the given service", func() {
		meshSvc := service.MeshService{
			Namespace: testNamespaceName,
			Name:      "test-GetBackpressurePolicy",
		}
		backpressurePolicy := &osmPolicy.Backpressure{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "policy.openservicemesh.io/v1alpha1",
				Kind:       "Backpressure",
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespaceName,
				Name:      "test-GetBackpressurePolicy",
			},
			Spec: osmPolicy.BackpressureSpec{
				MaxConnections: 123,
			},
		}

		_, err := fakeClientSet.osmPolicyClientSet.PolicyV1alpha1().Backpressures(testNamespaceName).Create(context.TODO(), backpressurePolicy, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()

		backpressurePolicyInCache := meshSpec.GetBackpressurePolicy(meshSvc)
		Expect(backpressurePolicyInCache).To(BeNil())

		err = fakeClientSet.osmPolicyClientSet.PolicyV1alpha1().Backpressures(testNamespaceName).Delete(context.TODO(), backpressurePolicy.Name, metav1.DeleteOptions{})
		Expect(err).ToNot(HaveOccurred())
		<-meshSpec.GetAnnouncementsChannel()
	})
})

var _ = Describe("When fetching the announcement channel", func() {
	var (
		meshSpec MeshSpec
		err      error
	)

	BeforeEach(func() {
		meshSpec, _, err = bootstrapClient()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return an announcement channel on which events are notified", func() {
		announcementChan := meshSpec.GetAnnouncementsChannel()
		Expect(announcementChan).ToNot(BeNil())
	})
})
