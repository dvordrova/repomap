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
    window.scrollTo({top:Math.max(0,window.scrollY+node.getBoundingClientRect().top-inset-16),behavior:'instant'});
  });});
}

// The page's data (page_data_table.go): the values an element's attribute
// refers to by index (data-reading="12"), each written once; the
// declarations a reading names, once, by index in its "decls"; and the base
// of the source links, once, a link inside the data starting with \u0001
// and one in a key attribute with "@". rmPage.data(element, name) is the
// value its dataset[name] refers to, parsed once, or null.
var rmPage = (function () {
  var page=null,values=new Map(),decls=new Map(),named={reading:1,inputPath:1,catalogue:1,launch:1};
  function load(){
    if(!page){var node=document.getElementById('rm-page-data');page=node?JSON.parse(node.textContent):{decls:[],values:[]};page.base=page.base||'';}
    return page;
  }
  function expand(value){
    if(typeof value==='string')return value.charCodeAt(0)===1?load().base+value.slice(1):value;
    if(Array.isArray(value))return value.map(expand);
    if(value&&typeof value==='object'){var out={};Object.keys(value).forEach(function(key){out[key]=expand(value[key]);});return out;}
    return value;
  }
  function decl(index){if(!decls.has(index))decls.set(index,expand(load().decls[index]));return Object.assign({},decls.get(index));}
  function data(element,name){
    var ref=element&&element.dataset?element.dataset[name]:undefined;
    if(ref===undefined||ref==='')return null;
    var key=name+'\0'+ref;
    if(!values.has(key)){
      var value=expand(load().values[Number(ref)]);
      if(named[name]&&value&&Array.isArray(value.decls))value.decls=value.decls.map(function(index){return typeof index==='number'?decl(index):index;});
      values.set(key,value);
    }
    return values.get(key);
  }
  function link(value){return value&&value.charAt(0)==='@'&&load().base?load().base+value.slice(1):value;}
  return {data:data,link:link};
})();
