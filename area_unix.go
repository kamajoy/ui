// +build !windows,!darwin,!plan9

// 14 march 2014

package ui

import (
	"unsafe"
	"image"
"fmt"
)

// #cgo pkg-config: gtk+-3.0
// #include "gtk_unix.h"
// extern gboolean our_area_draw_callback(GtkWidget *, cairo_t *, gpointer);
// extern gboolean our_area_button_press_event_callback(GtkWidget *, GdkEvent *, gpointer);
// extern gboolean our_area_button_release_event_callback(GtkWidget *, GdkEvent *, gpointer);
// extern gboolean our_area_motion_notify_event_callback(GtkWidget *, GdkEvent *, gpointer);
// extern gboolean our_area_key_press_event_callback(GtkWidget *, GdkEvent *, gpointer);
// extern gboolean our_area_key_release_event_callback(GtkWidget *, GdkEvent *, gpointer);
// /* HACK - see https://code.google.com/p/go/issues/detail?id=7548 */
// struct _cairo {};
import "C"

func gtkAreaNew() *gtkWidget {
	drawingarea := C.gtk_drawing_area_new()
	C.gtk_widget_set_size_request(drawingarea, 320, 240)
	// we need to explicitly subscribe to mouse events with GtkDrawingArea
	C.gtk_widget_add_events(drawingarea,
		C.GDK_BUTTON_PRESS_MASK | C.GDK_BUTTON_RELEASE_MASK | C.GDK_POINTER_MOTION_MASK | C.GDK_BUTTON_MOTION_MASK)
	// and we need to allow focusing on a GtkDrawingArea to enable keyboard events
	C.gtk_widget_set_can_focus(drawingarea, C.TRUE)
	scrollarea := C.gtk_scrolled_window_new((*C.GtkAdjustment)(nil), (*C.GtkAdjustment)(nil))
	// need a viewport because GtkDrawingArea isn't natively scrollable
	C.gtk_scrolled_window_add_with_viewport((*C.GtkScrolledWindow)(unsafe.Pointer(scrollarea)), drawingarea)
	return fromgtkwidget(scrollarea)
}

func gtkAreaGetControl(scrollarea *gtkWidget) *gtkWidget {
	viewport := C.gtk_bin_get_child((*C.GtkBin)(unsafe.Pointer(scrollarea)))
	control := C.gtk_bin_get_child((*C.GtkBin)(unsafe.Pointer(viewport)))
	return fromgtkwidget(control)
}

//export our_area_draw_callback
func our_area_draw_callback(widget *C.GtkWidget, cr *C.cairo_t, data C.gpointer) C.gboolean {
	var x, y, w, h C.double

	s := (*sysData)(unsafe.Pointer(data))
	// thanks to desrt in irc.gimp.net/#gtk+
	C.cairo_clip_extents(cr, &x, &y, &w, &h)
	cliprect := image.Rect(int(x), int(y), int(w), int(h))
	i := s.handler.Paint(cliprect)
	// pixel order is [R G B A] (see Example 1 on https://developer.gnome.org/gdk-pixbuf/2.26/gdk-pixbuf-The-GdkPixbuf-Structure.html) so we don't have to convert anything
	// gdk-pixbuf is not alpha-premultiplied (thanks to desrt in irc.gimp.net/#gtk+)
	pixbuf := C.gdk_pixbuf_new_from_data(
		(*C.guchar)(unsafe.Pointer(&i.Pix[0])),
		C.GDK_COLORSPACE_RGB,
		C.TRUE,			// has alpha channel
		8,				// bits per sample
		C.int(i.Rect.Dx()),
		C.int(i.Rect.Dy()),
		C.int(i.Stride),
		nil, nil)			// do not free data
	C.gdk_cairo_set_source_pixbuf(cr,
		pixbuf,
		C.gdouble(cliprect.Min.X),
		C.gdouble(cliprect.Min.Y))
	// that just set the brush that cairo uses: we have to actually draw now
	// (via https://developer.gnome.org/gtkmm-tutorial/stable/sec-draw-images.html.en)
	C.cairo_rectangle(cr, x, y, w, h)		// breaking the nrom here since we have the double data already
	C.cairo_fill(cr)
	C.g_object_unref((C.gpointer)(unsafe.Pointer(pixbuf)))		// free pixbuf
	return C.FALSE		// signals handled without stopping the event chain (thanks to desrt again)
}

var area_draw_callback = C.GCallback(C.our_area_draw_callback)

func translateModifiers(state C.guint, window *C.GdkWindow) C.guint {
	// GDK doesn't initialize the modifier flags fully; we have to explicitly tell it to (thanks to Daniel_S and daniels (two different people) in irc.gimp.net/#gtk+)
	C.gdk_keymap_add_virtual_modifiers(
		C.gdk_keymap_get_for_display(C.gdk_window_get_display(window)),
		(*C.GdkModifierType)(unsafe.Pointer(&state)))
	return state
}

func makeModifiers(state C.guint) (m Modifiers) {
	if (state & C.GDK_CONTROL_MASK) != 0 {
		m |= Ctrl
	}
	if (state & C.GDK_META_MASK) != 0 {
		m |= Alt
	}
	if (state & C.GDK_SHIFT_MASK) != 0 {
		m |= Shift
	}
	return m
}

// shared code for finishing up and sending a mouse event
func finishMouseEvent(data C.gpointer, me MouseEvent, mb uint, x C.gdouble, y C.gdouble, state C.guint, gdkwindow *C.GdkWindow) {
	s := (*sysData)(unsafe.Pointer(data))
	state = translateModifiers(state, gdkwindow)
	me.Modifiers = makeModifiers(state)
	// the mb != # checks exclude the Up/Down button from Held
	if mb != 1 && (state & C.GDK_BUTTON1_MASK) != 0 {
		me.Held = append(me.Held, 1)
	}
	if mb != 2 && (state & C.GDK_BUTTON2_MASK) != 0 {
		me.Held = append(me.Held, 2)
	}
	if mb != 3 && (state & C.GDK_BUTTON3_MASK) != 0 {
		me.Held = append(me.Held, 3)
	}
	// TODO keep?
	if mb != 4 && (state & C.GDK_BUTTON4_MASK) != 0 {
		me.Held = append(me.Held, 4)
	}
	if mb != 5 && (state & C.GDK_BUTTON5_MASK) != 0 {
		me.Held = append(me.Held, 5)
	}
	me.Pos = image.Pt(int(x), int(y))
	s.handler.Mouse(me)
}

//export our_area_button_press_event_callback
func our_area_button_press_event_callback(widget *C.GtkWidget, event *C.GdkEvent, data C.gpointer) C.gboolean {
	e := (*C.GdkEventButton)(unsafe.Pointer(event))
	me := MouseEvent{
		// GDK button ID == our button ID
		Down:	uint(e.button),
	}
	switch e._type {
	case C.GDK_BUTTON_PRESS:
		me.Count = 1
	case C.GDK_2BUTTON_PRESS:
		me.Count = 2
	default:		// ignore triple-clicks and beyond; we don't handle those
		return C.FALSE		// TODO really false?
	}
	finishMouseEvent(data, me, me.Down, e.x, e.y, e.state, e.window)
	return C.FALSE			// TODO really false?
}

var area_button_press_event_callback = C.GCallback(C.our_area_button_press_event_callback)

//export our_area_button_release_event_callback
func our_area_button_release_event_callback(widget *C.GtkWidget, event *C.GdkEvent, data C.gpointer) C.gboolean {
	e := (*C.GdkEventButton)(unsafe.Pointer(event))
	me := MouseEvent{
		// GDK button ID == our button ID
		Up:		uint(e.button),
	}
	finishMouseEvent(data, me, me.Up, e.x, e.y, e.state, e.window)
	return C.FALSE			// TODO really false?
}

var area_button_release_event_callback = C.GCallback(C.our_area_button_release_event_callback)

//export our_area_motion_notify_event_callback
func our_area_motion_notify_event_callback(widget *C.GtkWidget, event *C.GdkEvent, data C.gpointer) C.gboolean {
	e := (*C.GdkEventMotion)(unsafe.Pointer(event))
	me := MouseEvent{}
	finishMouseEvent(data, me, 0, e.x, e.y, e.state, e.window)
	return C.FALSE			// TODO really false?
}

var area_motion_notify_event_callback = C.GCallback(C.our_area_motion_notify_event_callback)

// shared code for doing a key event
func doKeyEvent(event *C.GdkEvent, data C.gpointer, up bool) bool {
	var ke KeyEvent

	e := (*C.GdkEventKey)(unsafe.Pointer(event))
	s := (*sysData)(unsafe.Pointer(data))
	keyval := e.keyval
	if extkey, ok := extkeys[keyval]; ok {
		ke.ExtKey = extkey
	} else if predef, ok := predefkeys[keyval]; ok {
		ke.ASCII = predef
	} else if _, ok := modonlykeys[keyval]; !ok {		// use ok form here to save memory/avoid racy map write
		cp := C.gdk_keyval_to_unicode(keyval)
		// GDK keycodes in GDK 3.4 the ASCII plane map to their ASCII values
		// (proof: https://git.gnome.org/browse/gtk+/tree/gdk/gdkkeysyms.h?h=gtk-3-4)
		// this also handles the numeric keypad keys (proof: https://git.gnome.org/browse/gtk+/tree/gdk/gdkkeyuni.c?h=gtk-3-4#n846)
		// the cp < 0x20 will also handle the case where the key is totally unknown to us (gdk_keyval_to_unicode() returns 0) and the space key
		if cp < 0x20 || cp >= 0x7F {
			// TODO really stop here? or should we handle modifiers?
			return false		// pretend unhandled
		}
		ke.ASCII = byte(cp)
	}
	state := translateModifiers(e.state, e.window)
	ke.Modifiers = makeModifiers(state)
	ke.Up = up
	return s.handler.Key(ke)
}

//export our_area_key_press_event_callback
func our_area_key_press_event_callback(widget *C.GtkWidget, event *C.GdkEvent, data C.gpointer) C.gboolean {
/*
	fmt.Printf("PRESS %#v\n", e)
	fmt.Printf("this (%d/GDK_KEY_%s):\n", e.keyval,
		C.GoString((*C.char)(unsafe.Pointer(
			C.gdk_keyval_name(e.keyval)))))
	pk(e.keyval, e.window)
	fmt.Printf("%d/GDK_KEY_A:\n", C.GDK_KEY_A)
	pk(C.GDK_KEY_A, e.window)
	fmt.Printf("%d/GDK_KEY_a:\n", C.GDK_KEY_a)
	pk(C.GDK_KEY_a, e.window)
*/
	ret := doKeyEvent(event, data, false)
	_ = ret
	return C.FALSE			// TODO really false? should probably return !ret (since true indicates stop processing)
}

var area_key_press_event_callback = C.GCallback(C.our_area_key_press_event_callback)

//export our_area_key_release_event_callback
func our_area_key_release_event_callback(widget *C.GtkWidget, event *C.GdkEvent, data C.gpointer) C.gboolean {
	ret := doKeyEvent(event, data, true)
	_ = ret
	return C.FALSE			// TODO really false? should probably return !ret (since true indicates stop processing)
}

var area_key_release_event_callback = C.GCallback(C.our_area_key_release_event_callback)

/*
func pk(keyval C.guint, window *C.GdkWindow) {
	var kk *C.GdkKeymapKey
	var nk C.gint

	km := C.gdk_keymap_get_for_display(C.gdk_window_get_display(window))
	b := C.gdk_keymap_get_entries_for_keyval(km, keyval, &kk, &nk)
	if b == C.FALSE {
		fmt.Println("(no key equivalent)")
		return
	}
	ok := kk
	for i := C.gint(0); i < nk; i++ {
		fmt.Printf("equiv %d/%d: %#v\n", i + 1, nk, kk)
		xkk := uintptr(unsafe.Pointer(kk))
		xkk += unsafe.Sizeof(kk)
		kk = (*C.GdkKeymapKey)(unsafe.Pointer(xkk))
	}
	C.g_free(C.gpointer(unsafe.Pointer(ok)))
}
*/

var extkeys = map[C.guint]ExtKey{
	C.GDK_KEY_Escape:			Escape,
	C.GDK_KEY_Insert:			Insert,
	C.GDK_KEY_Delete:			Delete,
	C.GDK_KEY_Home:			Home,
	C.GDK_KEY_End:			End,
	C.GDK_KEY_Page_Up:		PageUp,
	C.GDK_KEY_Page_Down:		PageDown,
	C.GDK_KEY_Up:			Up,
	C.GDK_KEY_Down:			Down,
	C.GDK_KEY_Left:			Left,
	C.GDK_KEY_Right:			Right,
	C.GDK_KEY_F1:			F1,
	C.GDK_KEY_F2:			F2,
	C.GDK_KEY_F3:			F3,
	C.GDK_KEY_F4:			F4,
	C.GDK_KEY_F5:			F5,
	C.GDK_KEY_F6:			F6,
	C.GDK_KEY_F7:			F7,
	C.GDK_KEY_F8:			F8,
	C.GDK_KEY_F9:			F9,
	C.GDK_KEY_F10:			F10,
	C.GDK_KEY_F11:			F11,
	C.GDK_KEY_F12:			F12,
	// numeric keypad equivalents:
	C.GDK_KEY_KP_Insert:		Insert,
	C.GDK_KEY_KP_Delete:		Delete,
	C.GDK_KEY_KP_Home:		Home,
	C.GDK_KEY_KP_End:		End,
	C.GDK_KEY_KP_Page_Up:		PageUp,
	C.GDK_KEY_KP_Page_Down:	PageDown,
	C.GDK_KEY_KP_Up:			Up,
	C.GDK_KEY_KP_Down:		Down,
	C.GDK_KEY_KP_Left:		Left,
	C.GDK_KEY_KP_Right:		Right,
}

// sanity check
func init() {
	included := make([]bool, _nextkeys)
	for _, v := range extkeys {
		included[v] = true
	}
	for i := 1; i < int(_nextkeys); i++ {
		if !included[i] {
			panic(fmt.Errorf("error: not all ExtKeys defined on Unix (missing %d)", i))
		}
	}
}

var predefkeys = map[C.guint]byte{
	C.GDK_KEY_Return:			'\n',
	// TODO C.GDK_KEY_Linefeed too? What key is this?
	C.GDK_KEY_Tab:			'\t',
	C.GDK_KEY_BackSpace:		'\b',
	// tests indicate that this is sent on Shift+Tab
	C.GDK_KEY_ISO_Left_Tab:	'\t',
	// numeric keypad equivalents:
	C.GDK_KEY_KP_Enter:		'\n',
	// all other numeric keypad equivalents are handled by gdk_keymap_to_unicode() as mentioned above
	// no space; handled by the code above
}

var modonlykeys =  map[C.guint]bool{
	C.GDK_KEY_Shift_L:		true,
	C.GDK_KEY_Shift_R:		true,
	C.GDK_KEY_Control_L:	true,
	C.GDK_KEY_Control_R:	true,
	C.GDK_KEY_Meta_L:		true,
	C.GDK_KEY_Meta_R:	true,
	// TODO GDK_KEY_Alt_L/R too?
	C.GDK_KEY_Super_L:	true,
	C.GDK_KEY_Super_R:	true,
}
