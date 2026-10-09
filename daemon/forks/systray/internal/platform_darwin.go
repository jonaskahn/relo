//go:build darwin

package internal

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/go-webgpu/goffi/ffi"

	"github.com/gogpu/systray/internal/darwin"
)

// NSVariableStatusItemLength tells NSStatusBar to size the item to fit its content.
const nsVariableStatusItemLength = -1.0

// menuItemCallbackID is the base for menu item command IDs.
// Each menu item gets baseID + index to route action callbacks.
const menuItemCallbackBaseID = 1000

// Selectors used by the darwin tray implementation.
// Registered lazily on first use.
var darwinSels struct {
	once sync.Once

	// NSObject
	alloc   darwin.SEL
	init    darwin.SEL
	release darwin.SEL

	// NSApplication
	sharedApplication       darwin.SEL
	setActivationPolicy     darwin.SEL
	run                     darwin.SEL
	stop                    darwin.SEL
	finishLaunching         darwin.SEL
	setApplicationIconImage darwin.SEL
	nextEventMatchingMask   darwin.SEL // nextEventMatchingMask:untilDate:inMode:dequeue:
	sendEvent               darwin.SEL

	// NSStatusBar
	systemStatusBar   darwin.SEL
	statusItemWithLen darwin.SEL // statusItemWithLength:
	removeStatusItem  darwin.SEL // removeStatusItem:

	// NSStatusItem
	button              darwin.SEL
	setMenu             darwin.SEL // setMenu:
	popUpStatusItemMenu darwin.SEL // popUpStatusItemMenu:

	// NSStatusBarButton (NSButton subclass)
	setImage       darwin.SEL // setImage:
	setToolTip     darwin.SEL // setToolTip:
	setTarget      darwin.SEL // setTarget:
	setAction      darwin.SEL // setAction:
	sendActionOn   darwin.SEL // sendActionOn:
	processInfo    darwin.SEL // processInfo
	setProcessName darwin.SEL // setProcessName:
	mainMenu       darwin.SEL // mainMenu
	submenu        darwin.SEL // submenu
	itemAtIndex    darwin.SEL // itemAtIndex:
	numberOfItems  darwin.SEL // numberOfItems
	action         darwin.SEL // action
	removeItem     darwin.SEL // removeItem:

	// NSImage
	initWithData darwin.SEL // initWithData:
	setSize      darwin.SEL // setSize:
	setTemplate  darwin.SEL // setTemplate:

	// NSMenu
	initWithTitle               darwin.SEL
	addItem                     darwin.SEL // addItem:
	separatorItem               darwin.SEL
	setSubmenu                  darwin.SEL // setSubmenu:
	initWithTitleActionKeyEquiv darwin.SEL // initWithTitle:action:keyEquivalent:
	setState                    darwin.SEL // setState:
	setEnabled                  darwin.SEL // setEnabled:
	setHidden                   darwin.SEL // setHidden:

	// NSObject main-thread dispatch
	performSelectorOnMainThread darwin.SEL // performSelectorOnMainThread:withObject:waitUntilDone:

	// NSDate
	distantPast   darwin.SEL
	distantFuture darwin.SEL

	// NSUserNotificationCenter
	defaultUserNotificationCenter darwin.SEL
	deliverNotification           darwin.SEL // deliverNotification:

	// NSUserNotification
	setTitle           darwin.SEL // setTitle:
	setInformativeText darwin.SEL // setInformativeText:
}

// Classes used by the darwin tray implementation.
var darwinClasses struct {
	once sync.Once

	NSApplication            darwin.Class
	NSStatusBar              darwin.Class
	NSImage                  darwin.Class
	NSMenu                   darwin.Class
	NSMenuItem               darwin.Class
	NSDate                   darwin.Class
	NSAutoreleasePool        darwin.Class
	NSUserNotificationCenter darwin.Class
	NSUserNotification       darwin.Class
	NSProcessInfo            darwin.Class
}

func initDarwinSels() {
	darwinSels.once.Do(func() {
		// NSObject
		darwinSels.alloc = darwin.RegisterSelector("alloc")
		darwinSels.init = darwin.RegisterSelector("init")
		darwinSels.release = darwin.RegisterSelector("release")

		// NSApplication
		darwinSels.sharedApplication = darwin.RegisterSelector("sharedApplication")
		darwinSels.setActivationPolicy = darwin.RegisterSelector("setActivationPolicy:")
		darwinSels.run = darwin.RegisterSelector("run")
		darwinSels.stop = darwin.RegisterSelector("stop:")
		darwinSels.finishLaunching = darwin.RegisterSelector("finishLaunching")
		darwinSels.setApplicationIconImage = darwin.RegisterSelector("setApplicationIconImage:")
		darwinSels.nextEventMatchingMask = darwin.RegisterSelector(
			"nextEventMatchingMask:untilDate:inMode:dequeue:")
		darwinSels.sendEvent = darwin.RegisterSelector("sendEvent:")

		// NSStatusBar
		darwinSels.systemStatusBar = darwin.RegisterSelector("systemStatusBar")
		darwinSels.statusItemWithLen = darwin.RegisterSelector("statusItemWithLength:")
		darwinSels.removeStatusItem = darwin.RegisterSelector("removeStatusItem:")

		// NSStatusItem
		darwinSels.button = darwin.RegisterSelector("button")
		darwinSels.setMenu = darwin.RegisterSelector("setMenu:")
		darwinSels.popUpStatusItemMenu = darwin.RegisterSelector("popUpStatusItemMenu:")

		// NSStatusBarButton (NSButton)
		darwinSels.setImage = darwin.RegisterSelector("setImage:")
		darwinSels.setToolTip = darwin.RegisterSelector("setToolTip:")
		darwinSels.setTarget = darwin.RegisterSelector("setTarget:")
		darwinSels.setAction = darwin.RegisterSelector("setAction:")
		darwinSels.sendActionOn = darwin.RegisterSelector("sendActionOn:")
		darwinSels.processInfo = darwin.RegisterSelector("processInfo")
		darwinSels.setProcessName = darwin.RegisterSelector("setProcessName:")
		darwinSels.mainMenu = darwin.RegisterSelector("mainMenu")
		darwinSels.submenu = darwin.RegisterSelector("submenu")
		darwinSels.itemAtIndex = darwin.RegisterSelector("itemAtIndex:")
		darwinSels.numberOfItems = darwin.RegisterSelector("numberOfItems")
		darwinSels.action = darwin.RegisterSelector("action")
		darwinSels.removeItem = darwin.RegisterSelector("removeItem:")

		// NSImage
		darwinSels.initWithData = darwin.RegisterSelector("initWithData:")
		darwinSels.setSize = darwin.RegisterSelector("setSize:")
		darwinSels.setTemplate = darwin.RegisterSelector("setTemplate:")

		// NSMenu
		darwinSels.initWithTitle = darwin.RegisterSelector("initWithTitle:")
		darwinSels.addItem = darwin.RegisterSelector("addItem:")
		darwinSels.separatorItem = darwin.RegisterSelector("separatorItem")
		darwinSels.setSubmenu = darwin.RegisterSelector("setSubmenu:")
		darwinSels.initWithTitleActionKeyEquiv = darwin.RegisterSelector(
			"initWithTitle:action:keyEquivalent:")
		darwinSels.setState = darwin.RegisterSelector("setState:")
		darwinSels.setEnabled = darwin.RegisterSelector("setEnabled:")
		darwinSels.setHidden = darwin.RegisterSelector("setHidden:")
		darwinSels.performSelectorOnMainThread = darwin.RegisterSelector(
			"performSelectorOnMainThread:withObject:waitUntilDone:")

		// NSDate
		darwinSels.distantPast = darwin.RegisterSelector("distantPast")
		darwinSels.distantFuture = darwin.RegisterSelector("distantFuture")

		// NSUserNotificationCenter
		darwinSels.defaultUserNotificationCenter = darwin.RegisterSelector(
			"defaultUserNotificationCenter")
		darwinSels.deliverNotification = darwin.RegisterSelector("deliverNotification:")

		// NSUserNotification
		darwinSels.setTitle = darwin.RegisterSelector("setTitle:")
		darwinSels.setInformativeText = darwin.RegisterSelector("setInformativeText:")
	})
}

func initDarwinClasses() {
	darwinClasses.once.Do(func() {
		darwinClasses.NSApplication = darwin.GetClass("NSApplication")
		darwinClasses.NSStatusBar = darwin.GetClass("NSStatusBar")
		darwinClasses.NSImage = darwin.GetClass("NSImage")
		darwinClasses.NSMenu = darwin.GetClass("NSMenu")
		darwinClasses.NSMenuItem = darwin.GetClass("NSMenuItem")
		darwinClasses.NSDate = darwin.GetClass("NSDate")
		darwinClasses.NSAutoreleasePool = darwin.GetClass("NSAutoreleasePool")
		darwinClasses.NSUserNotificationCenter = darwin.GetClass("NSUserNotificationCenter")
		darwinClasses.NSUserNotification = darwin.GetClass("NSUserNotification")
		darwinClasses.NSProcessInfo = darwin.GetClass("NSProcessInfo")
	})
}

// darwinTray implements PlatformTray using NSStatusBar/NSStatusItem.
type darwinTray struct {
	statusBar  darwin.ID // [NSStatusBar systemStatusBar]
	statusItem darwin.ID // NSStatusItem
	btn        darwin.ID // NSStatusBarButton (from [statusItem button])
	nsMenu     darwin.ID // NSMenu attached to the status item
	target     darwin.ID // GoSystrayTarget instance for click action routing

	callbacks *Callbacks
	iconData  []byte // stored PNG for recovery after Hide/Show

	// menuActions maps menu item indices to their callbacks.
	// Populated when SetMenu builds the NSMenu hierarchy.
	menuActions    map[int]func()
	nsItems        map[uint32]darwin.ID // item.ID() -> NSMenuItem handle (incl. submenus)
	menuMu         sync.Mutex
	pendingUpdates map[uint32]menuItemSnapshot
	destroying     bool
	destroyed      bool
}

// goSystrayTargetClass is the custom ObjC class registered once for click handling.
var (
	goSystrayTargetClass     darwin.Class
	goSystrayButtonClass     darwin.Class
	goSystrayTargetClassOnce sync.Once
	errGoSystrayTargetClass  error
)

// trayRegistry maps GoSystrayTarget ObjC instance pointer to the owning darwinTray.
// In ObjC callbacks, the `self` parameter identifies which target fired,
// allowing correct routing when multiple trays exist.
var (
	trayRegistryMu  sync.RWMutex
	trayRegistryMap = make(map[uintptr]*darwinTray)
	buttonRegistry  = make(map[uintptr]*darwinTray)
	runningNSApp    darwin.ID
)

// NewPlatformTray creates a macOS system tray implementation.
func NewPlatformTray(callbacks *Callbacks) PlatformTray {
	return &darwinTray{
		callbacks:      callbacks,
		menuActions:    make(map[int]func()),
		nsItems:        make(map[uint32]darwin.ID),
		pendingUpdates: make(map[uint32]menuItemSnapshot),
	}
}

// ensureNSApplicationLaunched initializes the shared NSApplication and
// finishes launching it before any NSStatusItem is created. Creating a
// status item before the app launch completes crashes with a
// "CGSConnectionByID" assertion (known macOS pattern: status items must not
// be created before applicationDidFinishLaunching). All steps are idempotent,
// safe to call from Create() and Run().
func ensureNSApplicationLaunched() {
	initDarwinSels()
	initDarwinClasses()

	// Get or create the shared NSApplication.
	nsApp := darwinClasses.NSApplication.Send(darwinSels.sharedApplication)
	if nsApp.IsNil() {
		return
	}

	// Finish launching is required before the event loop can process events
	// and before the window-server connection is available for UI objects.
	nsApp.Send(darwinSels.finishLaunching)
}

// Create initializes the NSStatusBar item and sets up click handling.
func (t *darwinTray) Create() error {
	// NSApplication must be launched before creating the status item;
	// otherwise AppKit aborts in CGSConnectionByID when the process has no
	// window-server connection yet.
	ensureNSApplicationLaunched()

	// Get the system status bar.
	t.statusBar = darwinClasses.NSStatusBar.Send(darwinSels.systemStatusBar)
	if t.statusBar.IsNil() {
		return errors.New("darwin: failed to get NSStatusBar")
	}

	// Create status item with variable length.
	// [statusBar statusItemWithLength:NSVariableStatusItemLength]
	t.statusItem = t.statusBar.SendDouble(darwinSels.statusItemWithLen, nsVariableStatusItemLength)
	if t.statusItem.IsNil() {
		return errors.New("darwin: failed to create NSStatusItem")
	}

	// Get the button associated with the status item.
	t.btn = t.statusItem.Send(darwinSels.button)
	if t.btn.IsNil() {
		return errors.New("darwin: NSStatusItem has no button")
	}

	// Register custom ObjC target class for click action routing.
	targetClass, err := registerGoSystrayTarget()
	if err != nil {
		return fmt.Errorf("darwin: register target class: %w", err)
	}

	// Create an instance of the target and set it on the button.
	t.target = darwin.ID(targetClass).Send(darwinSels.alloc)
	t.target = t.target.Send(darwinSels.init)
	if t.target.IsNil() {
		return errors.New("darwin: failed to create GoSystrayTarget")
	}
	if darwin.SetObjectClass(t.btn, goSystrayButtonClass) == 0 {
		return errors.New("darwin: failed to install status button click handler")
	}

	// Register in tray registry so ObjC callbacks can route to this instance.
	trayRegistryMu.Lock()
	trayRegistryMap[t.target.Ptr()] = t
	buttonRegistry[t.btn.Ptr()] = t
	trayRegistryMu.Unlock()

	// Set the button's target to our GoSystrayTarget instance and its action
	// to the trayClicked: selector. When the user clicks the status item
	// button, the ObjC runtime sends trayClicked: to our target.
	t.btn.SendPtr(darwinSels.setTarget, t.target.Ptr())
	trayClickedSel := darwin.RegisterSelector("trayClicked:")
	t.btn.SendPtr(darwinSels.setAction, uintptr(trayClickedSel))
	t.btn.SendInt(darwinSels.sendActionOn, 1<<2)

	return nil
}

// registerGoSystrayTarget creates a custom ObjC class "GoSystrayTarget" that
// handles button click actions. The class is created once and reused.
func registerGoSystrayTarget() (darwin.Class, error) {
	goSystrayTargetClassOnce.Do(func() {
		nsObjectClass := darwin.GetClass("NSObject")
		if nsObjectClass == 0 {
			errGoSystrayTargetClass = darwin.ErrClassNotFound
			return
		}

		cls := darwin.AllocateClassPair(nsObjectClass, "GoSystrayTarget")
		if cls == 0 {
			errGoSystrayTargetClass = errors.New("darwin: failed to allocate GoSystrayTarget class")
			return
		}

		// Add trayClicked: method — called when the status bar button is clicked.
		// ObjC signature: -(void)trayClicked:(id)sender → "v@:@"
		trayClickedIMP := ffi.NewCallback(func(self, sel, sender uintptr) uintptr {
			trayRegistryMu.RLock()
			t := trayRegistryMap[self]
			trayRegistryMu.RUnlock()
			if t != nil && t.callbacks != nil {
				if fn := t.callbacks.OnClick; fn != nil {
					fn()
				}
			}
			return 0
		})
		darwin.ClassAddMethod(cls, darwin.RegisterSelector("trayClicked:"), trayClickedIMP, "v@:@")

		// Add menuItemClicked: method — called when a menu item is clicked.
		// We use the sender's tag to look up the Go callback.
		// ObjC signature: -(void)menuItemClicked:(NSMenuItem*)sender → "v@:@"
		menuClickedIMP := ffi.NewCallback(func(self, sel, sender uintptr) uintptr {
			trayRegistryMu.RLock()
			t := trayRegistryMap[self]
			trayRegistryMu.RUnlock()
			if t == nil {
				return 0
			}
			tagSel := darwin.RegisterSelector("tag")
			tag := darwin.ID(sender).Send(tagSel)
			idx := int(tag)

			t.menuMu.Lock()
			fn := t.menuActions[idx]
			t.menuMu.Unlock()

			if fn != nil {
				// AppKit is tracking the menu on this thread. A callback that
				// waits here (the Quit item drains the daemon) keeps the menu
				// open and holds the removal until the next click.
				go fn()
			}
			return 0
		})
		darwin.ClassAddMethod(cls, darwin.RegisterSelector("menuItemClicked:"), menuClickedIMP, "v@:@")

		// Add drainUpdates: method — called on main thread via performSelectorOnMainThread.
		// Applies captured menu state on the main thread.
		drainUpdatesIMP := ffi.NewCallback(func(self, sel, sender uintptr) uintptr {
			trayRegistryMu.RLock()
			t := trayRegistryMap[self]
			trayRegistryMu.RUnlock()
			if t != nil {
				t.applyPendingUpdates()
			}
			return 0
		})
		darwin.ClassAddMethod(cls, darwin.RegisterSelector("drainUpdates:"), drainUpdatesIMP, "v@:@")

		rightMouseDownIMP := ffi.NewCallback(func(self, sel, event uintptr) uintptr {
			trayRegistryMu.RLock()
			t := buttonRegistry[self]
			trayRegistryMu.RUnlock()
			if t != nil {
				if t.callbacks != nil {
					if fn := t.callbacks.OnRightClick; fn != nil {
						fn()
					}
				}
				_ = t.ShowMenu()
			}
			return 0
		})
		statusButtonClass := darwin.GetClass("NSStatusBarButton")
		if statusButtonClass == 0 {
			errGoSystrayTargetClass = darwin.ErrClassNotFound
			return
		}
		buttonClass := darwin.AllocateClassPair(statusButtonClass, "GoSystrayStatusBarButton")
		if buttonClass == 0 {
			errGoSystrayTargetClass = errors.New("darwin: failed to allocate status button class")
			return
		}
		if !darwin.ClassAddMethod(
			buttonClass,
			darwin.RegisterSelector("rightMouseDown:"),
			rightMouseDownIMP,
			"v@:@",
		) {
			errGoSystrayTargetClass = errors.New("darwin: failed to add right-click handler")
			return
		}
		darwin.RegisterClassPair(buttonClass)
		goSystrayButtonClass = buttonClass

		darwin.RegisterClassPair(cls)
		goSystrayTargetClass = cls
	})

	return goSystrayTargetClass, errGoSystrayTargetClass
}

// SetIcon sets the tray icon from PNG bytes.
// The image is resized to 22x22 points, the standard macOS menu bar icon size.
func (t *darwinTray) SetIcon(png []byte) error {
	if t.statusItem.IsNil() || t.btn.IsNil() {
		return errors.New("darwin: tray not created")
	}

	t.iconData = png

	nsImage := createNSImage(png, false)
	if nsImage.IsNil() {
		return errors.New("darwin: failed to create NSImage from PNG data")
	}

	// [button setImage:nsImage]
	t.btn.SendPtr(darwinSels.setImage, nsImage.Ptr())

	return nil
}

func (t *darwinTray) SetDockIcon(png []byte) error {
	image := createNSImage(png, false)
	if image.IsNil() {
		return errors.New("darwin: failed to create Dock icon")
	}
	app := darwinClasses.NSApplication.Send(darwinSels.sharedApplication)
	app.SendPtr(darwinSels.setApplicationIconImage, image.Ptr())
	return nil
}

// SetTemplateIcon sets a macOS template image. Template images are monochrome
// and the system automatically adjusts their appearance for the current menu
// bar style (light/dark).
func (t *darwinTray) SetTemplateIcon(png []byte) error {
	if t.statusItem.IsNil() || t.btn.IsNil() {
		return errors.New("darwin: tray not created")
	}

	t.iconData = png

	nsImage := createNSImage(png, true)
	if nsImage.IsNil() {
		return errors.New("darwin: failed to create template NSImage")
	}

	// [button setImage:nsImage]
	t.btn.SendPtr(darwinSels.setImage, nsImage.Ptr())

	return nil
}

// createNSImage creates an NSImage from PNG data, optionally marking it as
// a template image. The image is resized to 22x22 points.
func createNSImage(png []byte, template bool) darwin.ID {
	initDarwinSels()
	initDarwinClasses()

	// Create NSData from the raw PNG bytes.
	nsData := darwin.NewNSData(png)
	if nsData.IsNil() {
		return 0
	}

	// [[NSImage alloc] initWithData:nsData]
	nsImage := darwinClasses.NSImage.Send(darwinSels.alloc)
	if nsImage.IsNil() {
		return 0
	}
	nsImage = nsImage.SendPtr(darwinSels.initWithData, nsData.Ptr())
	if nsImage.IsNil() {
		return 0
	}

	// [nsImage setSize:NSMakeSize(22, 22)] — standard menu bar icon size
	nsImage.SendSize(darwinSels.setSize, darwin.NSSize{Width: 22, Height: 22})

	// [nsImage setTemplate:YES] if requested
	if template {
		nsImage.SendBool(darwinSels.setTemplate, true)
	}

	return nsImage
}

// SetTooltip sets the hover tooltip text.
func (t *darwinTray) SetTooltip(text string) error {
	if t.btn.IsNil() {
		return errors.New("darwin: tray not created")
	}

	nsStr := darwin.NewNSString(text)
	if nsStr.IsNil() {
		return errors.New("darwin: failed to create NSString for tooltip")
	}

	// [button setToolTip:nsString]
	t.btn.SendPtr(darwinSels.setToolTip, nsStr.Ptr())

	return nil
}

func (t *darwinTray) SetAppName(name string) {
	process := darwinClasses.NSProcessInfo.Send(darwinSels.processInfo)
	value := darwin.NewNSString(name)
	if !process.IsNil() && !value.IsNil() {
		process.SendPtr(darwinSels.setProcessName, value.Ptr())
	}
}

func (t *darwinTray) RemoveAppMenuRoles(roles ...string) {
	app := darwinClasses.NSApplication.Send(darwinSels.sharedApplication)
	main := app.Send(darwinSels.mainMenu)
	if main.IsNil() {
		return
	}
	root := main.SendInt(darwinSels.itemAtIndex, 0)
	menu := root.Send(darwinSels.submenu)
	if menu.IsNil() {
		return
	}
	remove := make(map[darwin.SEL]struct{}, len(roles))
	for _, role := range roles {
		remove[darwin.RegisterSelector(role)] = struct{}{}
	}
	for index := int(menu.Send(darwinSels.numberOfItems)) - 1; index >= 0; index-- {
		item := menu.SendInt(darwinSels.itemAtIndex, int64(index))
		action := darwin.SEL(item.Send(darwinSels.action).Ptr())
		if _, ok := remove[action]; ok {
			menu.SendPtr(darwinSels.removeItem, item.Ptr())
		}
	}
}

// SetMenu builds an NSMenu from our Menu struct and attaches it to the status item.
func (t *darwinTray) SetMenu(menu *Menu) error {
	if t.statusItem.IsNil() {
		return errors.New("darwin: tray not created")
	}

	if menu == nil {
		// Remove the menu. When no menu is set, the button action (trayClicked:)
		// fires on click.
		t.statusItem.SendPtr(darwinSels.setMenu, 0)
		t.nsMenu = 0
		return nil
	}

	// Build the NSMenu hierarchy.
	t.menuMu.Lock()
	// Clear old actions and item handle mappings.
	t.menuActions = make(map[int]func())
	t.nsItems = make(map[uint32]darwin.ID)
	t.menuMu.Unlock()

	counter := menuItemCallbackBaseID
	nsMenu := t.buildNSMenu("", menu, &counter)
	if nsMenu.IsNil() {
		return errors.New("darwin: failed to build NSMenu")
	}

	t.nsMenu = nsMenu

	return nil
}

func (t *darwinTray) ShowMenu() error {
	if t.statusItem.IsNil() || t.nsMenu.IsNil() {
		return nil
	}
	// AppKit's nested tracking loop bypasses the outer event pump. Service
	// captured state there too, so disabled rows and removal update while open.
	timer := darwin.NewTimer(0.02, t.target, darwin.RegisterSelector("drainUpdates:"))
	runLoop := darwin.ID(darwin.GetClass("NSRunLoop")).Send(darwin.RegisterSelector("currentRunLoop"))
	common := darwin.NewNSString("kCFRunLoopCommonModes")
	darwin.MsgSendPtrPtr(runLoop, darwin.RegisterSelector("addTimer:forMode:"), timer.Ptr(), common.Ptr())
	common.Send(darwinSels.release)
	defer timer.Send(darwin.RegisterSelector("invalidate"))
	t.statusItem.SendPtr(darwinSels.popUpStatusItemMenu, t.nsMenu.Ptr())
	return nil
}

// buildNSMenu recursively converts a Menu into an NSMenu.
// counter is incremented per item and used as the tag for callback routing.
func (t *darwinTray) buildNSMenu(title string, menu *Menu, counter *int) darwin.ID {
	initDarwinSels()
	initDarwinClasses()

	// Create NSMenu.
	nsMenu := darwinClasses.NSMenu.Send(darwinSels.alloc)
	if nsMenu.IsNil() {
		return 0
	}

	if title != "" {
		nsTitle := darwin.NewNSString(title)
		nsMenu = nsMenu.SendPtr(darwinSels.initWithTitle, nsTitle.Ptr())
	} else {
		nsMenu = nsMenu.Send(darwinSels.init)
	}
	if nsMenu.IsNil() {
		return 0
	}

	nsMenu.SendBool(darwin.RegisterSelector("setAutoenablesItems:"), false)
	menuClickedSel := darwin.RegisterSelector("menuItemClicked:")

	for _, item := range menu.Items {
		snapshot := item.snapshot()
		switch snapshot.itemType {
		case MenuItemSeparator:
			sep := darwinClasses.NSMenuItem.Send(darwinSels.separatorItem)
			if !sep.IsNil() {
				sep.SendBool(darwinSels.setHidden, snapshot.hidden)
				nsMenu.SendPtr(darwinSels.addItem, sep.Ptr())
			}

		case MenuItemSubmenu:
			// Create a placeholder NSMenuItem for the submenu.
			nsLabel := darwin.NewNSString(snapshot.label)
			emptyKey := darwin.NewNSString("")
			nsItem := darwinClasses.NSMenuItem.Send(darwinSels.alloc)
			nsItem = darwin.MsgSend3Ptr(nsItem, darwinSels.initWithTitleActionKeyEquiv,
				nsLabel.Ptr(), 0, emptyKey.Ptr())
			if nsItem.IsNil() {
				continue
			}
			nsItem.SendBool(darwinSels.setHidden, snapshot.hidden)

			// Build the submenu recursively.
			subMenu := t.buildNSMenu(snapshot.label, item.Submenu, counter)
			if !subMenu.IsNil() {
				nsItem.SendPtr(darwinSels.setSubmenu, subMenu.Ptr())
			}

			nsMenu.SendPtr(darwinSels.addItem, nsItem.Ptr())

			// Store the submenu container handle for dynamic updates.
			// Submenu containers are NSMenuItems like any other — users can
			// call SetLabel/SetDisabled on the *MenuItem returned by
			// AddSubmenu, so it must be resolvable via nsItems.
			t.menuMu.Lock()
			t.nsItems[snapshot.id] = nsItem
			t.menuMu.Unlock()

		default:
			// Normal or checkbox item.
			idx := *counter
			*counter++

			nsLabel := darwin.NewNSString(snapshot.label)
			emptyKey := darwin.NewNSString("")
			nsItem := darwinClasses.NSMenuItem.Send(darwinSels.alloc)

			// Set action to menuItemClicked: on our target.
			nsItem = darwin.MsgSend3Ptr(nsItem, darwinSels.initWithTitleActionKeyEquiv,
				nsLabel.Ptr(), uintptr(menuClickedSel), emptyKey.Ptr())
			if nsItem.IsNil() {
				continue
			}
			nsItem.SendBool(darwinSels.setHidden, snapshot.hidden)

			// Set the target so Cocoa sends the action to our GoSystrayTarget.
			nsItem.SendPtr(darwinSels.setTarget, t.target.Ptr())

			// Set tag for callback routing.
			// [nsItem setTag:idx]
			setTagSel := darwin.RegisterSelector("setTag:")
			nsItem.SendInt(setTagSel, int64(idx))

			// Set checked state for checkbox items.
			// NSControlStateValueOn = 1, NSControlStateValueOff = 0
			if snapshot.itemType == MenuItemCheckbox && snapshot.checked {
				nsItem.SendInt(darwinSels.setState, 1)
			}

			// Set icon if provided.
			if len(snapshot.icon) > 0 {
				nsImage := createNSImage(snapshot.icon, false)
				if !nsImage.IsNil() {
					nsItem.SendPtr(darwinSels.setImage, nsImage.Ptr())
				}
			}

			// Register Go callback and map item ID to NSMenuItem handle for
			// UpdateItem lookup. itemWithTag: only searches the root menu, so
			// items inside submenus must be resolved via this map.
			t.menuMu.Lock()
			if item.OnClick != nil {
				t.menuActions[idx] = item.OnClick
			}
			t.nsItems[snapshot.id] = nsItem
			t.menuMu.Unlock()

			nsMenu.SendPtr(darwinSels.addItem, nsItem.Ptr())
		}
	}

	return nsMenu
}

// UpdateItem dispatches a menu item update to the main thread.
// AppKit requires all UI mutations on the main thread. We enqueue the snapshot
// and call performSelectorOnMainThread to drain the queue safely.
func (t *darwinTray) UpdateItem(item *MenuItem) error {
	return t.updateItem(item.snapshot())
}

func (t *darwinTray) updateItem(item menuItemSnapshot) error {
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	if t.destroying || t.destroyed || t.target.IsNil() {
		return nil
	}
	if _, ok := t.nsItems[item.id]; !ok {
		return nil
	}
	t.pendingUpdates[item.id] = item
	t.dispatchPendingUpdates()
	return nil
}

func (t *darwinTray) dispatchPendingUpdates() {
	// Common modes include menu tracking; no mouse event should be needed to drain.
	common := darwin.NewNSString("kCFRunLoopCommonModes")
	modes := darwin.ID(darwin.GetClass("NSArray")).SendPtr(darwin.RegisterSelector("arrayWithObject:"), common.Ptr())
	darwin.MsgSend4Ptr(t.target, darwin.RegisterSelector("performSelectorOnMainThread:withObject:waitUntilDone:modes:"),
		uintptr(darwin.RegisterSelector("drainUpdates:")), 0, 0, modes.Ptr())
	common.Send(darwinSels.release)
	trayRegistryMu.RLock()
	nsApp := runningNSApp
	trayRegistryMu.RUnlock()
	darwin.PostAppDefinedEvent(nsApp)
}

func (t *darwinTray) applyPendingUpdates() {
	t.menuMu.Lock()
	if t.destroyed {
		t.menuMu.Unlock()
		return
	}
	if t.destroying {
		t.destroyed = true
		t.pendingUpdates = nil
		t.menuMu.Unlock()
		t.destroyOnMainThread()
		return
	}
	updates := t.pendingUpdates
	t.pendingUpdates = make(map[uint32]menuItemSnapshot)
	t.menuMu.Unlock()
	for _, item := range updates {
		t.applyItemUpdate(item)
	}
}

// applyItemUpdate applies a single snapshot to its NSMenuItem.
// MUST be called on the main thread.
func (t *darwinTray) applyItemUpdate(item menuItemSnapshot) {
	t.menuMu.Lock()
	nsItem, ok := t.nsItems[item.id]
	t.menuMu.Unlock()
	if !ok || nsItem.IsNil() {
		return
	}

	nsTitle := darwin.NewNSString(item.label)
	if !nsTitle.IsNil() {
		nsItem.SendPtr(darwinSels.setTitle, nsTitle.Ptr())
	}

	if item.itemType == MenuItemCheckbox {
		state := int64(0)
		if item.checked {
			state = 1
		}
		nsItem.SendInt(darwinSels.setState, state)
	}

	nsItem.SendBool(darwinSels.setEnabled, !item.disabled)
	nsItem.SendBool(darwinSels.setHidden, item.hidden)

	if len(item.icon) > 0 {
		nsImage := createNSImage(item.icon, true)
		if !nsImage.IsNil() {
			nsImage.SendSize(darwinSels.setSize, darwin.NSSize{Width: 16, Height: 16})
			nsItem.SendPtr(darwinSels.setImage, nsImage.Ptr())
			nsImage.Send(darwinSels.release)
		}
	}
}

// ShowNotification displays an OS-level notification using NSUserNotification.
// NSUserNotification was deprecated in macOS 10.14 in favor of UNUserNotification,
// but remains functional through at least macOS 14. A future version may migrate
// to UNUserNotificationCenter.
func (t *darwinTray) ShowNotification(title, message string) error {
	initDarwinSels()
	initDarwinClasses()

	// Create NSUserNotification.
	notification := darwinClasses.NSUserNotification.Send(darwinSels.alloc)
	notification = notification.Send(darwinSels.init)
	if notification.IsNil() {
		return errors.New("darwin: failed to create NSUserNotification")
	}

	// Set title.
	nsTitle := darwin.NewNSString(title)
	if !nsTitle.IsNil() {
		notification.SendPtr(darwinSels.setTitle, nsTitle.Ptr())
	}

	// Set informative text (body).
	nsMessage := darwin.NewNSString(message)
	if !nsMessage.IsNil() {
		notification.SendPtr(darwinSels.setInformativeText, nsMessage.Ptr())
	}

	// Deliver via the default notification center.
	center := darwinClasses.NSUserNotificationCenter.Send(darwinSels.defaultUserNotificationCenter)
	if center.IsNil() {
		return errors.New("darwin: failed to get NSUserNotificationCenter")
	}

	center.SendPtr(darwinSels.deliverNotification, notification.Ptr())

	return nil
}

// Show makes the tray icon visible. The status item is visible immediately
// after Create(), so this is effectively a no-op unless Hide() was called.
func (t *darwinTray) Show() error {
	if !t.statusItem.IsNil() {
		// Already visible.
		return nil
	}

	// Re-create the status item if it was removed by Hide().
	if t.statusBar.IsNil() {
		return errors.New("darwin: tray not created")
	}

	t.statusItem = t.statusBar.SendDouble(darwinSels.statusItemWithLen, nsVariableStatusItemLength)
	if t.statusItem.IsNil() {
		return errors.New("darwin: failed to re-create NSStatusItem")
	}

	t.btn = t.statusItem.Send(darwinSels.button)

	// Restore icon if we had one.
	if len(t.iconData) > 0 {
		if err := t.SetIcon(t.iconData); err != nil {
			slog.Warn("darwin: failed to restore icon after Show", "err", err)
		}
	}

	// Restore menu if we had one.
	if !t.nsMenu.IsNil() {
		t.statusItem.SendPtr(darwinSels.setMenu, t.nsMenu.Ptr())
	}

	// Restore target/action for click handling.
	if !t.target.IsNil() && !t.btn.IsNil() {
		t.btn.SendPtr(darwinSels.setTarget, t.target.Ptr())
		trayClickedSel := darwin.RegisterSelector("trayClicked:")
		t.btn.SendPtr(darwinSels.setAction, uintptr(trayClickedSel))
	}

	return nil
}

// Hide removes the status item from the menu bar without destroying the tray.
// Call Show() to make it visible again.
func (t *darwinTray) Hide() error {
	if t.statusBar.IsNil() || t.statusItem.IsNil() {
		return nil
	}

	// [statusBar removeStatusItem:statusItem]
	t.statusBar.SendPtr(darwinSels.removeStatusItem, t.statusItem.Ptr())
	t.statusItem = 0
	t.btn = 0

	return nil
}

// Bounds returns the tray icon's screen position.
// On macOS, NSStatusItem does not provide a direct API for this.
// Returns zeros; callers should not depend on this for positioning.
func (t *darwinTray) Bounds() (int, int, int, int) {
	// NSStatusItem window frame could be queried via [[[statusItem button] window] frame],
	// but this requires NSRect return handling. For v1, return zeros.
	return 0, 0, 0, 0
}

// Run pumps Cocoa events and pending tray changes on the calling main thread.
// It returns after the last tray's Destroy() has run.
// Only call Run() once per process: the shared NSApplication event loop serves
// all tray icons.
func (t *darwinTray) Run() error {
	initDarwinSels()
	initDarwinClasses()

	// Get or create the shared NSApplication.
	nsApp := darwinClasses.NSApplication.Send(darwinSels.sharedApplication)
	if nsApp.IsNil() {
		return errors.New("darwin: failed to get NSApplication")
	}
	trayRegistryMu.Lock()
	runningNSApp = nsApp
	trayRegistryMu.Unlock()

	// Finish launching is required before the event loop can process events.
	nsApp.Send(darwinSels.finishLaunching)

	// AppKit's selector queue was not serviced by NSApp.run through Go FFI.
	// Drain from the event pump itself so removal never needs another click.
	mode := darwin.NewNSString("kCFRunLoopDefaultMode")
	defer mode.Send(darwinSels.release)
	for drainDarwinTrays() {
		pool := darwinClasses.NSAutoreleasePool.Send(darwinSels.alloc).Send(darwinSels.init)
		event := darwin.MsgSend4Ptr(nsApp, darwinSels.nextEventMatchingMask, ^uintptr(0), darwinClasses.NSDate.Send(darwinSels.distantFuture).Ptr(), mode.Ptr(), 1)
		remaining := drainDarwinTrays()
		if remaining && !event.IsNil() {
			nsApp.SendPtr(darwinSels.sendEvent, event.Ptr())
		}
		pool.Send(darwin.RegisterSelector("drain"))
		if !remaining {
			break
		}
	}

	trayRegistryMu.Lock()
	if runningNSApp == nsApp {
		runningNSApp = 0
	}
	trayRegistryMu.Unlock()

	return nil
}

// Destroy queues main-thread removal without waiting on a menu or update queue.
// Removing the final tray wakes the event pump so Run returns without another click.
func (t *darwinTray) Destroy() {
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	if t.target.IsNil() || t.destroying || t.destroyed {
		return
	}
	t.destroying = true
	t.dispatchPendingUpdates()
}

// destroyOnMainThread performs the actual AppKit cleanup.
// MUST be called on the main thread.
func (t *darwinTray) destroyOnMainThread() {
	if !t.nsMenu.IsNil() {
		t.nsMenu.Send(darwin.RegisterSelector("cancelTracking"))
	}
	// Remove the status item from the menu bar.
	if !t.statusBar.IsNil() && !t.statusItem.IsNil() {
		t.statusBar.SendPtr(darwinSels.removeStatusItem, t.statusItem.Ptr())
	}

	// Unregister from tray registry before releasing the target. Only the final
	// tray owns shutdown of the shared application event loop.
	lastTray := false
	var nsApp darwin.ID
	if !t.target.IsNil() {
		trayRegistryMu.Lock()
		delete(trayRegistryMap, t.target.Ptr())
		delete(buttonRegistry, t.btn.Ptr())
		lastTray = shouldStopDarwinApplication(len(trayRegistryMap))
		nsApp = runningNSApp
		trayRegistryMu.Unlock()
	}

	// Release ObjC objects.
	t.menuMu.Lock()
	defer t.menuMu.Unlock()
	if !t.target.IsNil() {
		t.target.Send(darwinSels.release)
		t.target = 0
	}

	t.statusItem = 0
	t.btn = 0
	t.nsMenu = 0

	if lastTray && !nsApp.IsNil() {
		darwin.PostAppDefinedEvent(nsApp)
	}
}

func shouldStopDarwinApplication(remainingTrays int) bool {
	return remainingTrays == 0
}

func drainDarwinTrays() bool {
	trayRegistryMu.RLock()
	trays := make([]*darwinTray, 0, len(trayRegistryMap))
	for _, tray := range trayRegistryMap {
		trays = append(trays, tray)
	}
	trayRegistryMu.RUnlock()
	for _, tray := range trays {
		tray.applyPendingUpdates()
	}
	trayRegistryMu.RLock()
	defer trayRegistryMu.RUnlock()
	return len(trayRegistryMap) > 0
}
