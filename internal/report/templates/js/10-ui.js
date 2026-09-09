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
function rmLocalReadingActions(parent,explanation,toCode) {
  var actions=document.createElement('nav');actions.className='local-reading-actions';
  [['To explanation',function(){explanation.scrollIntoView({block:'nearest'});}],['To code',toCode]].forEach(function(item){
    var button=document.createElement('button');button.type='button';button.textContent=rmT(item[0]);button.addEventListener('click',item[1]);actions.appendChild(button);
  });parent.appendChild(actions);return actions;
}
