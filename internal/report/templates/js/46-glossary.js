// The report owns term mentions, including translated forms and their exact
// definition IDs. The browser only applies those spans to unchanged prose.
(function () {
  var catalogue=document.getElementById('concepts'),data=document.getElementById('rm-term-mentions');
  if(!catalogue||!data?.textContent.trim())return;
  var plans;try{plans=JSON.parse(data.textContent);}catch(error){return;}
  if(!Array.isArray(plans)||!plans.length)return;
  var entries=new Map(),byRef=new Map(),boundTerms=new WeakSet();
  catalogue.querySelectorAll('.learn-concept').forEach(function(entry){entries.set(entry.id,entry);});
  plans.forEach(function(plan){
    if(typeof plan.ref!=='string'||!plan.ref||typeof plan.text!=='string'||!Array.isArray(plan.spans))return;
    byRef.set(plan.ref,plan);
  });
  var card=document.createElement('aside');card.className='source-card term-popup';card.id='rm-term-preview';
  card.setAttribute('role','dialog');card.setAttribute('aria-modal','false');card.setAttribute('aria-labelledby','rm-term-preview-title');card.tabIndex=-1;
  var selector='[data-display-ref]';
  var excluded='a,button,code,pre,textarea,input,select,summary,h1,h2,h3,h4,h5,h6,svg,.anchor,.chip,.path,.route-path,.route-symbol,.reading-excerpt,.reading-members,.reading-source,.call-path,.learn-concept,.answer-term,.term-popup,.source-card:not(.map-card),[contenteditable="true"]';
  function textNodes(element){
    var nodes=[],text='',walker=document.createTreeWalker(element,NodeFilter.SHOW_TEXT),node;
    while((node=walker.nextNode())){
      nodes.push({node:node,start:text.length,end:text.length+node.data.length,eligible:!node.parentElement.closest(excluded)});
      text+=node.data;
    }
    return {nodes:nodes,text:text};
  }
  function copyDefinition(entry){
    var item=document.createElement('article');item.className='term-popup-definition';
    var title=document.createElement('h3');title.textContent=entry.dataset.termName;
    if(entry.dataset.termOriginal&&entry.dataset.termOriginal!==entry.dataset.termName){var original=document.createElement('span');original.className='meta';original.textContent=' · '+entry.dataset.termOriginal;title.appendChild(original);}
    item.appendChild(title);
    var content=entry.querySelector('.glossary-entry-content').cloneNode(true);
    content.querySelectorAll('[id]').forEach(function(node){node.removeAttribute('id');});
    item.appendChild(content);
    var link=document.createElement('a');link.className='term-popup-glossary';link.href='#'+entry.id;link.textContent=rmT('Open in glossary')+' →';item.appendChild(link);
    return item;
  }
  function bindTerm(trigger,ids){
    var binding;
    boundTerms.add(trigger);
    trigger.setAttribute('aria-controls',card.id);trigger.setAttribute('aria-haspopup','dialog');trigger.setAttribute('aria-expanded','false');
    trigger.setAttribute('aria-label',rmT('Explain {0}',trigger.textContent));
    binding=repomapPreview.bind(trigger,card,function(){
      card.replaceChildren();
      var heading=document.createElement('div');heading.className='term-popup-heading';
      var title=document.createElement('strong');title.id='rm-term-preview-title';title.textContent=rmT('Term explanation');heading.appendChild(title);
      var close=document.createElement('button');close.type='button';close.className='term-popup-close';close.textContent=rmT('Close');close.addEventListener('click',function(){binding.hide(true);});heading.appendChild(close);card.appendChild(heading);
      var state=document.createElement('p');state.className='term-popup-state';state.textContent=rmT('Explanation pinned');card.appendChild(state);
      if(ids.length>1){var note=document.createElement('p');note.className='term-popup-variants';note.textContent=rmT('This name has {0} definitions in this report.',ids.length);card.appendChild(note);}
      ids.forEach(function(id){card.appendChild(copyDefinition(entries.get(id)));});
    },{pinOnClick:true,returnFocus:true,focusOnPin:true});
  }
  function annotate(element){
    if(!element.isConnected||element.closest(excluded)||element.querySelector('.term-mention'))return;
    var plan=byRef.get(element.dataset.displayRef);
    if(!plan)return;
    // The ref selects this exact prose slot. Text equality only verifies that
    // its offsets still describe the displayed bytes, including after cloning.
    var value=textNodes(element);
    if(plan.text!==value.text)return;
    var selected=[],seen=new Set(),previousParagraph=1,answer=element.closest('.reading-guide'),paragraph=answer||element.closest('p,li');
    if(paragraph&&paragraph!==element)paragraph.querySelectorAll('.term-mention').forEach(function(mention){seen.add(mention.dataset.termIds.split(' ').sort().join('|'));});
    plan.spans.forEach(function(span){
      if(!Number.isInteger(span.start)||!Number.isInteger(span.end)||span.start<0||span.end<=span.start||span.end>value.text.length||!Array.isArray(span.ids)||!span.ids.length||span.ids.some(function(id){return !entries.has(id);}))return;
      var parts=value.nodes.filter(function(row){return row.end>span.start&&row.start<span.end;});
      if(!parts.length||parts.some(function(row){return !row.eligible;}))return;
      var paragraph=value.text.slice(0,span.start).split(/\n\s*\n/).length;
      if(!answer&&paragraph!==previousParagraph){seen.clear();previousParagraph=paragraph;}
      // All source-distinct variants stay together. Repeating the same set in
      // one answer adds no new explanation and needs no second underline.
      var key=span.ids.slice().sort().join('|');if(seen.has(key))return;seen.add(key);
      selected.push({span:span,parts:parts});
    });
    selected.reverse().forEach(function(match){
      var first=match.parts[0],last=match.parts[match.parts.length-1],span=match.span;
      var range=document.createRange();range.setStart(first.node,span.start-first.start);range.setEnd(last.node,span.end-last.start);
      var trigger=document.createElement('button');trigger.type='button';trigger.className='term-mention';trigger.dataset.termIds=span.ids.join(' ');
      trigger.appendChild(range.extractContents());range.insertNode(trigger);bindTerm(trigger,span.ids);
    });
  }
  function scan(root){
    if(!(root instanceof Element)||root.closest('.term-mention,.term-popup,.learn-concept'))return;
    // cloneNode copies our visible buttons but cannot copy their listeners.
    // Restore their original text, then bind only this location's exact plan.
    root.querySelectorAll('.term-mention').forEach(function(mention){if(!boundTerms.has(mention))mention.replaceWith.apply(mention,Array.from(mention.childNodes));});
    if(root.matches(selector))annotate(root);
    root.querySelectorAll(selector).forEach(annotate);
  }
  var main=document.querySelector('main');if(!main)return;scan(main);
  // Maps replace selected descriptions in place. Observe only those reading
  // surfaces; building a popup and wrapping its own text never re-enter here.
  var pending=new Set(),scheduled=false;
  var observer=new MutationObserver(function(records){
    records.forEach(function(record){
      var parent=record.target.nodeType===Node.TEXT_NODE?record.target.parentElement:record.target;
      if(parent?.closest('.term-mention,.term-popup,.learn-concept'))return;
      var paragraph=parent?.closest(selector);if(paragraph)pending.add(paragraph);
      record.addedNodes.forEach(function(node){if(node.nodeType===Node.ELEMENT_NODE)pending.add(node);});
    });
    if(!pending.size||scheduled)return;scheduled=true;
    queueMicrotask(function(){scheduled=false;var roots=Array.from(pending);pending.clear();roots.forEach(scan);});
  });
  main.querySelectorAll('[data-map]').forEach(function(map){observer.observe(map,{childList:true,characterData:true,subtree:true});});
})();
