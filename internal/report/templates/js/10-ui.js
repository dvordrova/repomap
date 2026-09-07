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
