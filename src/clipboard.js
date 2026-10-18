
ObjC.import('AppKit');
function run() {
  const raw = $.NSProcessInfo.processInfo.environment.objectForKey('LEO_CLIP_ITEMS');
  if (raw.isNil()) return 'no-items';
  const items = JSON.parse(raw.js);
  const pb = $.NSPasteboard.generalPasteboard;
  pb.clearContents;
  const arr = $.NSMutableArray.alloc.init;
  for (let i = 0; i < items.length; i++) {
    const url = $.NSURL.fileURLWithPath(items[i].path);
    const item = $.NSPasteboardItem.alloc.init;
    item.setStringForType(url.absoluteString, 'public.file-url');
    if (items[i].type) {
      const data = $.NSData.dataWithContentsOfFile(items[i].path);
      if (!data.isNil()) item.setDataForType(data, items[i].type);
    }
    arr.addObject(item);
  }
  return pb.writeObjects(arr) ? 'ok' : 'write-failed';
}
