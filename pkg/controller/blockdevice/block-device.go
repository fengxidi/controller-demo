package blockdevice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/time/rate"

	block "controller-demo/pkg/apis/device/v1alpha1"
	"controller-demo/pkg/client/clientset/versioned"
	blockscheme "controller-demo/pkg/client/clientset/versioned/scheme"
	Informer "controller-demo/pkg/client/informers/externalversions"
	lister "controller-demo/pkg/client/listers/device/v1alpha1"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	covev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"k8s.io/client-go/kubernetes"
	eventv1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type blockDeviceController struct {
	Client       versioned.Interface
	kubeClient   kubernetes.Interface
	Listers      lister.BlockDeviceLister
	Recorder     record.EventRecorder
	Queue        workqueue.RateLimitingInterface
	DeviceSynced cache.InformerSynced
}

func NewBlockDeviceController(
	Client versioned.Interface,
	kubeClient kubernetes.Interface,
	Listers lister.BlockDeviceLister,
	blockinformer Informer.SharedInformerFactory) *blockDeviceController {
	// 将自定义的资源加入到 Scheme, 是为了能够进行系列化和反系列化，如果不添加，在系列化时会报错
	utilruntime.Must(blockscheme.AddToScheme(scheme.Scheme))
	klog.Info("createing event broadcaster")
	// 实例化一个事件广播器
	eventBroadcaster := record.NewBroadcaster()
	// 取消记录结构化事件,这里是通过将日志等级设置为0 来取消这个日志记录
	// 通俗的说就是，当controller创建事件时，需不需要将事件记录到日志中
	eventBroadcaster.StartStructuredLogging(0)
	// 将事件发送到发送到指定的接收器，这里的接收器指定为 k8s的 事件
	eventBroadcaster.StartRecordingToSink(&eventv1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	// 通过定义的事件广播器实例化一个事件记录器，后续需要记录事件时，就可以通过这个事件记录器进行了
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, covev1.EventSource{Component: "blockdevice-controller"})

	controller := &blockDeviceController{
		Client:     Client,
		kubeClient: kubeClient,
		Recorder:   recorder,
		Listers:    Listers,
		// 定义一个工作队列，该队列结合了延时队列和桶队列两种限速队列
		Queue: workqueue.NewNamedRateLimitingQueue( // Mixed speed limit treatment
			workqueue.NewMaxOfRateLimiter(
				workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 1*time.Minute), // Queuing sort
				&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},  // Token Bucket
			),
			"blockdevice-controller",
		),
		// 这里的 DeviceSynced 主要是通过listandwatch机制，判断资源是否同步完成，一般在启动控制器时，用于等待同步资源完成
		DeviceSynced: blockinformer.Suxueit().V1alpha1().BlockDevices().Informer().HasSynced,
	}

	klog.Info("setting up event handlers")
	// 这里是controller能够获取到资源变更的关键，注册了自定义的事件处理函数
	// 通过事件处理，将事件放入到队列中，后续通过从队列获取到事件key，从缓存中获取到key对应的资源对象，即可进行资源的同步
	blockinformer.Suxueit().V1alpha1().BlockDevices().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// 新增事件
		AddFunc: controller.AddenqueueFn, // 资源新增，在controller重启后，所有的已存在的blockdevice资源都会经过这里
		// 更新事件
		UpdateFunc: func(old, new interface{}) {
			newDevice := new.(*block.BlockDevice)
			oldDevice := old.(*block.BlockDevice)
			// 如果更新时两个前后版本资源版本相同，则调过，这里是有可能存在事件重复发送的问题，可以通过版本之间跳过处理
			if newDevice.ResourceVersion == oldDevice.ResourceVersion {
				return
			}
			controller.enqueueFn(new)
		},
		// 删除事件
		DeleteFunc: controller.enqueueFn,
	})

	return controller

}

func (ctrl *blockDeviceController) Run(threading int, stopCh <-chan struct{}) error {
	// 通过 recover 捕捉 ceash
	defer utilruntime.HandleCrash()
	// close the queue
	defer ctrl.Queue.ShutDown()

	klog.Info("starting block device controller")

	klog.V(2).Info("waiting for informer caches to sync")
	// 启动的时候会在这里等待资源同步完成
	if ok := cache.WaitForCacheSync(stopCh, ctrl.DeviceSynced); !ok {
		return fmt.Errorf("failed to wait for chaches to sync")
	}

	klog.V(2).Info("Starting workers")
	for i := 0; i < threading; i++ {
		// wait.Until 是k8s 的util提供的功能，主要作用是 间隔多长时间运行一次 runWorker
		//注： 如果上次的 worker还没有结束，下一次不会运行
		go wait.Until(ctrl.runWorker, time.Second, stopCh)
	}
	// 每分钟采集一次节点上的块设备
	go wait.Until(ctrl.SyncNodeBlockDevice, time.Minute, stopCh)
	// 通过channel阻塞协程
	<-stopCh
	klog.Info("shutting down  blockdevice-controller  workers")
	return nil

}

func (ctrl *blockDeviceController) runWorker() {
	// 遍历队列
	for ctrl.processNextWorkItem() {

	}
}

func (ctrl *blockDeviceController) processNextWorkItem() bool {
	// 从队列取值
	obj, shutdown := ctrl.Queue.Get()
	// Exit if the queue is colse
	if shutdown {
		return false
	}

	err := func(obj interface{}) error {
		// 从队首删除这个key
		defer ctrl.Queue.Done(obj)
		var key string
		var ok bool

		if key, ok = obj.(string); !ok {
			ctrl.Queue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in queue but got %#v", obj))
			return nil

		}

		// 开始同步资源
		if err := ctrl.Sync(key); err != nil {
			// Synchronization failed, rejoin the queue
			ctrl.Queue.AddRateLimited(obj)
			// 这个函数可以获取到 这个key在队列中本次处理失败的次数
			// num := ctrl.Queue.NumRequeues(obj)
			return fmt.Errorf("error syncing %s : %s, requeuing", key, err.Error())

		}
		klog.Infof("success sync %s ", key)
		// 当资源成功同步，将它从队列移除，【后续就需要等待下次有相关事件再入队进行处理】
		ctrl.Queue.Forget(obj)

		return nil
	}(obj)

	if err != nil {
		klog.Error(err.Error())

	}
	return true

}

// Sync Synchronize the  device state to the desired state
func (ctrl *blockDeviceController) Sync(key string) error {
	klog.Infof("Start synchronizing device %s", key)
	// 这个key的结构是 namespace/name : 例如： default/test
	// 由于blockdevice是一个集群资源，所以namespace始终是 空的
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		klog.Errorf("invalid resource key %s", key)
		return nil
	}
	// 从缓存中获取资源对象
	device, err := ctrl.Listers.Get(name)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			klog.Errorf("Work queue does not have device %s", name)
			return nil
		}
	}
	// 这里通过深拷贝拷贝一份对象，因为这里的对象都是指针对象，如果不深拷贝controller在修改时会直接到缓存，而一旦我们controller同步过程中出现错误
	// 然后修改已经生效到缓存，就会影响到下一次的同步
	newdevice := device.DeepCopy()

	// 去同步blockdevice资源
	if err = ctrl.syncBlockDevice(newdevice); err != nil {
		return fmt.Errorf("failed to update device %s", newdevice.Name)
	}

	klog.Infof("Successfully synchronized device %s", newdevice.Name)

	return nil
}

//syncBlockDevice  Synchronize the device state to the desired state
func (ctrl *blockDeviceController) syncBlockDevice(block *block.BlockDevice) (err error) {

	// block.Name = "NodeName-name"
	deviceName := strings.Split(block.Name, "-")
	// 这里将不符合规范的 设备【可能是手动创建的】删除
	if len(deviceName) < 2 {
		return ctrl.Client.SuxueitV1alpha1().BlockDevices().Delete(context.TODO(), block.Name, metav1.DeleteOptions{})
	}
	// 对于不是本节点的 设备不进行处理
	if block.Spec.Node != NodeName {
		return nil
	}

	// 同时更新设备的在线状态和更新时间[只针对没有更新过状态的 设备]
	if block.Status.Status == "" {
		block.Status.Status = Online
		block.Status.LastUpdateTime = metav1.Now()

		return ctrl.UpdateBlockDeviceStatus(block)
	}
	return

}

// AddenqueueFn 是因为这里需要在新增时，单独处理一下 设备管理器
func (ctrl *blockDeviceController) AddenqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		klog.Errorf("Invalid object:  %s", err.Error())
		return
	}
	klog.Infof("Listened to the block device change event, changed object %s", key)

	ctrl.Queue.Add(key)
	device := obj.(*block.BlockDevice)
	// 加入到设备管理器
	BlockDeviceManager[key] = Device{Name: device.Spec.Name, Size: device.Spec.Size, Type: device.Spec.Tyep}

}

// enqueueFn When creating a device, add the device to the queue
func (ctrl *blockDeviceController) enqueueFn(obj interface{}) {

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		klog.Errorf("Invalid object:  %s", err.Error())
		return
	}
	klog.Infof("Listened to the block device change event, changed object %s", key)

	ctrl.Queue.Add(key)

}

// 创建设备资源
func (ctrl blockDeviceController) CreateBlockDevice(name, devType, size string) error {
	deviceName := fmt.Sprintf("%s-%s", NodeName, name)
	device := block.BlockDevice{
		ObjectMeta: metav1.ObjectMeta{Name: deviceName},
		Spec: block.BlockDeviceSpec{
			Name: name,
			Tyep: devType,
			Size: size,
			Node: NodeName,
		},
	}
	_, err := ctrl.Client.SuxueitV1alpha1().BlockDevices().Create(context.TODO(), &device, metav1.CreateOptions{})
	if err != nil {
		klog.Error(err)
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil

}

// 更新设备资源
func (ctrl blockDeviceController) UpdateBlockDevice(device *block.BlockDevice) error {

	// 调用接口进行更新
	_, err := ctrl.Client.SuxueitV1alpha1().BlockDevices().Update(context.TODO(), device, metav1.UpdateOptions{})
	if err != nil {
		klog.Error(err)
		return err
	}
	return nil

}

// 删除设备资源【这里只针对 controller认为需要删除的设备】
func (ctrl blockDeviceController) DeleteBlockDevice(deviceName string) error {

	dev, err := ctrl.Listers.Get(deviceName)
	if err != nil {
		klog.Error(err)
		if k8serrors.IsNotFound(err) {
			// 如果不存在进行创建
			return nil
		}
		return err
	}
	newDev := dev.DeepCopy()
	now := metav1.Now()
	switch dev.Status.Status {
	case Online, "":
		// 如果需要删除的设备处于在线状态，则让设备离线
		newDev.Status.Status = OffLine
		newDev.Status.LastUpdateTime = now
		return ctrl.UpdateBlockDeviceStatus(newDev)

	case OffLine:
		// 如果 需要删除的设备 离线时间大于 1分钟，才删除
		fmt.Println(now, newDev.Name)
		if now.Sub(dev.Status.LastUpdateTime.Time) > time.Minute {
			return ctrl.Client.SuxueitV1alpha1().BlockDevices().Delete(context.TODO(), newDev.Name, metav1.DeleteOptions{})

		}
	}

	return nil

}

// 更新设备状态资源
func (ctrl blockDeviceController) UpdateBlockDeviceStatus(device *block.BlockDevice) error {

	// 调用接口进行更新
	_, err := ctrl.Client.SuxueitV1alpha1().BlockDevices().UpdateStatus(context.TODO(), device, metav1.UpdateOptions{})
	if err != nil {
		klog.Error(err)
		return err
	}
	return nil

}

func (ctrl blockDeviceController) CreateOrUpdateBlockDevice(name, devType, size string) error {
	klog.Infof("同步 %s 设备 %s", NodeName, name)

	deviceName := fmt.Sprintf("%s-%s", NodeName, name)
	BlockDeviceManager[deviceName] = Device{Name: name, Type: devType, Size: size}
	dev, err := ctrl.Listers.Get(deviceName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// 如果不存在进行创建
			return ctrl.CreateBlockDevice(name, devType, size)
		}
	}

	// 如果存在，但是值不一样，则进行更新
	if dev.Spec.Tyep != devType || dev.Spec.Size != size {
		newDev := dev.DeepCopy()
		newDev.Spec.Tyep = devType
		newDev.Spec.Size = size

		return ctrl.UpdateBlockDevice(newDev)
	}
	klog.Infof("同步 %s 设备 %s 成功", NodeName, name)

	return nil
}

// 同步 节点上的设备到 k8s 资源
func (ctrl blockDeviceController) SyncNodeBlockDevice() {
	// 获取块设备的路径
	klog.Info("同步块设备...")
	devicePathList := AllDevicePath()
	// 通过读取块设备路径下的文件，获取设备信息
	nameList := []string{}
	for _, pathDir := range devicePathList {
		name, devType, size, err := CollectBlockDevice(pathDir)
		if err != nil {
			klog.Error(err)
			continue
		}

		err = ctrl.CreateOrUpdateBlockDevice(name, devType, size)
		if err != nil {
			klog.Error(err)
			continue
		}
		deviceName := fmt.Sprintf("%s-%s", NodeName, name)
		nameList = append(nameList, deviceName)
	}

	// 将已经不存在的设备进行删除
	oldNameSet := make(map[string]struct{})
	for key := range BlockDeviceManager {
		oldNameSet[key] = struct{}{}
	}
	// 这样就可以得到 老设备列表和新设备列表的差集
	for _, v := range nameList {
		delete(oldNameSet, v)
	}

	for key := range oldNameSet {
		err := ctrl.DeleteBlockDevice(key)
		if err != nil {
			klog.Error(err)
			continue
		}
		delete(BlockDeviceManager, key)
	}

}
