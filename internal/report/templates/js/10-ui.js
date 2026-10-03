// The same report-owned vocabulary renders static HTML and interactive labels.
// Replacement never translates values: code names, source text and parameters
// are inserted once. The dictionary contains plain text, never trusted HTML.
var rmT = (function () {
  var node=document.getElementById('rm-ui-vocabulary');
  var vocabulary=node?JSON.parse(node.textContent):{};
  function text(key) {
    if(!Object.prototype.hasOwnProperty.call(vocabulary,key))throw new Error('Unknown report UI message: '+key);
    var args=Array.prototype.slice.call(arguments,1),used=new Set();
    var value=args.length&&String(args[0])==='1'&&Object.prototype.hasOwnProperty.call(vocabulary,key+'.one')?vocabulary[key+'.one']:vocabulary[key];
    var result=value.replace(/\{(\d+)\}/g,function(token,index){
      index=Number(index);if(index>=args.length)throw new Error('Missing report UI parameter: '+key);
      used.add(index);return String(args[index]);
    });
    if(used.size!==args.length)throw new Error('Extra report UI parameter: '+key);
    return result;
  }
  text.html=function(){var value=text.apply(null,arguments);return value.replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});};
  return text;
})();

// SVG hash anchors have no reliable scroll margin. Scroll the containing
// reading surface after its layout and the toolbar have settled instead.
function rmScrollToReading(node) {
  if(!node)return;
  if(node.matches('[data-node]'))node=node.closest('[data-map]');
  requestAnimationFrame(function(){requestAnimationFrame(function(){
    if(!node.isConnected||node.closest('[hidden]'))return;
    var toolbar=document.querySelector('.report-toolbar'),inset=toolbar&&getComputedStyle(toolbar).position==='sticky'?toolbar.getBoundingClientRect().height:0;
    // A surface already wholly in sight under the toolbar stays where it
    // is (freqtrade, 2026-10-02: a declaration chosen from an input's flow
    // moved the page 16 pixels under the reader's pointer).
    var at=node.getBoundingClientRect();
    if(at.top>=inset&&at.bottom<=window.innerHeight)return;
    window.scrollTo({top:Math.max(0,window.scrollY+at.top-inset-16),behavior:'instant'});
  });});
}

// The page's data (page_data_table.go), read back exactly as Go wrote each
// value: the values an element's attribute refers to by index
// (data-reading="12"), each written once; the declarations a reading names,
// once, by index in its "decls"; the base of the source links, once (a link
// inside the data starting with \u0001, one in a key attribute with "@");
// a link its place says written as 1 ("href":1 beside "at":"redis.c:9068"),
// a declaration's identity as the rest of its place (":5:function:main"
// beside "at":"redis.c:9068"),
// a link to all of a declaration's lines as its last line; a call's ends by
// their index in the declarations, a relation's names when they are its
// declarations' names and its kind when it is "calls";
// a tile's declaration by its index; a reading's call as a call unless it
// says otherwise; and any part written again elsewhere as {"$": index} into
// "shared". rmPage.data(element, name) is the value its dataset[name]
// refers to, read once, or null.
var rmPage = (function () {
  var page=null,values=new Map(),decls=new Map(),named={reading:1,inputPath:1,catalogue:1,launch:1,reached:1,files:1};
  function load(){
    if(!page){var node=document.getElementById('rm-page-data');page=node?JSON.parse(node.textContent):{decls:[],values:[]};page.base=page.base||'';page.range=page.range||'-L';page.shared=page.shared||[];}
    return page;
  }
  // A place as the page writes it: "path:line" or "path:line:column".
  function place(text){var at=/^(.*?):(\d+)(?::(\d+))?$/.exec(text);return at?{path:at[1],line:Number(at[2])}:{path:text,line:0};}
  function said(text){var at=place(text);return load().base+at.path+(at.line>0?'#L'+at.line:'');}
  var links=[['href','at','code'],['href','source','code'],['from','at',''],['Href','Text','Code']],keyPlaces=[['key','at'],['key','source'],['Key','Text']];
  function expand(value){
    if(typeof value==='string')return value.charCodeAt(0)===1?load().base+value.slice(1):value;
    if(Array.isArray(value))return value.map(expand);
    if(value&&typeof value==='object'){
      var keys=Object.keys(value);
      if(keys.length===1&&keys[0]==='$')return expand(load().shared[value.$]);
      var out={};keys.forEach(function(key){out[key]=expand(value[key]);});
      links.forEach(function(pair){
        var text=out[pair[1]];if(typeof text!=='string'||!text)return;
        if(out[pair[0]]===1)out[pair[0]]=said(text);
        if(pair[1]==='Text'&&!('Open' in out))out.Open='';
        if(pair[2]&&typeof out[pair[2]]==='number'&&place(text).line>0)out[pair[2]]=said(text)+load().range+out[pair[2]];
      });
      // A declaration's identity written as the rest of its place.
      keyPlaces.forEach(function(pair){
        var key=out[pair[0]],text=out[pair[1]];
        if(typeof key==='string'&&key.charAt(0)===':'&&typeof text==='string'&&text)out[pair[0]]=text+key;
      });
      if(typeof out.decl_key==='string'&&out.decl_key.charAt(0)===':'&&out.path)out.decl_key=out.path+':'+out.line+out.decl_key;
      return out;
    }
    return value;
  }
  function decl(index){if(!decls.has(index))decls.set(index,expand(load().decls[index]));return Object.assign({},decls.get(index));}
  function declKey(index){var d=decl(index);return d.key||d.href;}
  // A call's ends by index. A relation between two named ends (a call with
  // neither words nor a handler's name) gets back its kind, "calls" unless
  // it says otherwise, and the names its declarations say.
  function call(item){
    var caller=typeof item.caller==='number'?item.caller:-1,callee=typeof item.callee==='number'?item.callee:typeof item.to==='number'?item.to:-1;
    if(!('label' in item)&&!('name' in item)){
      if(!('kind' in item))item.kind='calls';
      if(!('caller_name' in item)&&caller>=0)item.caller_name=decl(caller).name;
      if(!('callee_name' in item)&&callee>=0)item.callee_name=decl(callee).name;
    }
    if(caller>=0)item.caller=declKey(caller);
    if(typeof item.callee==='number')item.callee=declKey(item.callee);
    // A call landing where its callee is declared: its link and its
    // callee's identity, both from that declaration.
    if(typeof item.to==='number'){item.callee=declKey(item.to);item.to=decl(item.to).href;}
  }
  // A tile's declaration by index: its identity, name, link, code and file.
  function symbol(item){
    if(typeof item.d!=='number')return;
    var d=decl(item.d);item.name=d.name;item.href=d.href;if(d.code!==undefined)item.code=d.code;if(d.key!==undefined)item.decl_key=d.key;item.path=place(d.at||d.source).path;delete item.d;
  }
  // A reading's relation ends are calls unless they say otherwise.
  function kinds(value){
    if(Array.isArray(value)){value.forEach(kinds);return;}
    if(!value||typeof value!=='object')return;
    Object.keys(value).forEach(function(key){
      var item=value[key];
      if(Array.isArray(item)&&(key==='ends'||key==='decls'))item.forEach(function(end){if(end&&typeof end==='object'&&!('kind' in end))end.kind='calls';});
      kinds(item);
    });
  }
  function data(element,name){
    var ref=element&&element.dataset?element.dataset[name]:undefined;
    if(ref===undefined||ref==='')return null;
    var key=name+'\0'+ref;
    if(!values.has(key)){
      var value=expand(load().values[Number(ref)]);
      if(name==='calls'&&Array.isArray(value))value.forEach(function(item){if(item&&typeof item==='object')call(item);});
      if(name==='symbols'&&Array.isArray(value))value.forEach(function(item){if(item&&typeof item==='object')symbol(item);});
      if(name==='reading')kinds(value);
      if(named[name]&&value&&Array.isArray(value.decls))value.decls=value.decls.map(function(index){return typeof index==='number'?decl(index):index;});
      values.set(key,value);
    }
    return values.get(key);
  }
  function link(value){return value&&value.charAt(0)==='@'&&load().base?load().base+value.slice(1):value;}
  // The static link to a file's line (0: the whole file), "" when the page
  // has no static links.
  function lineLink(path,line){var base=load().base;return base?base+path+(line>0?'#L'+line:''):'';}
  return {data:data,link:link,lineLink:lineLink};
})();
