// Find existing descriptions and source entries, then open their actual map.
// This is a local index of the complete HTML, not another interpretation pass.
(function () {
  var nav=document.querySelector('nav.nav');
  if(!nav)return;
  var box=document.createElement('input');box.type='search';box.className='find';
  box.placeholder=rmT('Find');box.setAttribute('aria-label',rmT('Find in repository'));
  var panel=document.createElement('section');panel.className='find-panel';panel.hidden=true;panel.id='repository-find';
  panel.setAttribute('aria-label',rmT('Repository search results'));box.setAttribute('aria-controls',panel.id);
  panel.innerHTML=("<div class=\"find-tools\"><label>"+rmT.html("Show")+" <select data-find-kind aria-label=\""+rmT.html("Search result type")+"\"><option value=\"all\">"+rmT.html("Everything")+"</option><option value=\"question\">"+rmT.html("Questions and answers")+"</option><option value=\"term\">"+rmT.html("Terms")+"</option><option value=\"part\">"+rmT.html("Parts")+"</option><option value=\"operation\">"+rmT.html("Operations")+"</option><option value=\"code\">"+rmT.html("Code")+"</option><option value=\"external\">"+rmT.html("External communication")+"</option></select></label><label>"+rmT.html("In")+" <select data-find-component aria-label=\""+rmT.html("Search component")+"\"><option value=\"\">"+rmT.html("All components")+"</option></select></label><button type=\"button\" data-close>"+rmT.html("Close")+"</button></div><p class=\"find-status\" role=\"status\"></p><ol class=\"find-results\"></ol><div class=\"find-pages\"><button type=\"button\" data-prev>"+rmT.html("← Previous")+"</button><span></span><button type=\"button\" data-next>"+rmT.html("Next →")+"</button></div>");
  nav.insertBefore(box,nav.querySelector('.repomap-link'));nav.appendChild(panel);
  box.focusSearch=function(){box.focus({preventScroll:true});return box;};
  var kind=panel.querySelector('[data-find-kind]'),component=panel.querySelector('[data-find-component]'),status=panel.querySelector('.find-status'),results=panel.querySelector('ol'),pages=panel.querySelector('.find-pages'),page=0,pageSize=12;
  var entries=[],components={},groupNodes={},codeEntries=new Map(),lastQuery='';
  function modelText(node){if(!node)return '';var copy=node.cloneNode(true);copy.querySelectorAll('.source-hint,.model-sources').forEach(function(n){n.remove();});return copy.textContent;}
  function add(entry){entry.title=entry.title||'';entry.summary=entry.summary||'';entry.path=entry.path||'';entry.haystack=(entry.title+' '+entry.summary+' '+entry.path+' '+entry.component+' '+(entry.additionalText||'')).toLowerCase();entries.push(entry);}
  // What a map node is to a reader looking for it: its search kind and the
  // word its result shows. A component is listed once, below. An outside
  // call or its destination is not a part of the program (gethostbyname was
  // listed as a Part three times), and an input collection is its inputs.
  function nodeKind(d){
    if(d.branch==='component')return null;
    if(d.activation)return {kind:'operation',type:rmT(d.activation)};
    if(d.itemKind==='External communication')return {kind:'external',type:rmT('External communication')};
    if(d.branch==='inputs')return {kind:'operation',type:rmT('Inputs')};
    return {kind:'part',type:rmT(d.branch?'Area':'Part')};
  }
  // An input the model did not explain is read by its handler, never by its
  // registration's raw call words.
  function nodeSummary(d){return d.summary||(d.handler&&d.handler!==d.title?rmT('handled by')+' '+d.handler:d.declaredBy?rmT('Declared in {0}',d.declaredBy):'');}
  // A declaration off the map stays findable by its name: it opens its code
  // and the component's list of what is not on the map.
  document.querySelectorAll('[data-system-map] [data-branch="component"]').forEach(function(n){
    var id=(n.getAttribute('href')||'').slice(1);if(!id)return;
    components[id]=n.dataset.title;
    add({title:n.dataset.title,summary:n.dataset.summary,component:n.dataset.title,section:id,kind:'part',type:rmT('Component'),destination:document.getElementById(id)});
  });
  Object.keys(components).sort(function(a,b){return components[a].localeCompare(components[b],document.documentElement.lang);}).forEach(function(id){var option=document.createElement('option');option.value=id;option.textContent=components[id];component.appendChild(option);});
  document.querySelectorAll('[data-map-explorer] [data-node]').forEach(function(n){
    if(n.dataset.remote==='true')return;
    var map=n.closest('[data-map-explorer]');
    if(map.displayedNode(n)!==n)return;
    var section=document.getElementById(n.dataset.owner)||n.closest('section'),id=section.id,href=n.getAttribute('href');
    if(href&&href[0]==='#'&&!n.dataset.activation&&!n.dataset.branch)groupNodes[href.slice(1)]=n;
    var found=nodeKind(n.dataset);if(!found)return;
    add({title:n.dataset.title,summary:nodeSummary(n.dataset),additionalText:map.areaDescriptions(n).concat(n.dataset.declaredBy?[n.dataset.declaredBy]:[]).join(' '),component:components[id]||(section.querySelector('h2')||section.querySelector('h3')||n).textContent||n.dataset.title,section:id,
      kind:found.kind,type:found.type,node:n,map:map});
  });
  // One entry per declaration, by its file and line: adlist.c's listCreate,
  // compiled into three programs, was three results with nothing telling
  // them apart. Each program that holds it is one "In program / part →"
  // link; a program that leaves it off its map links to the list saying so.
  function programOf(section){return components[section.id]||(section.querySelector('h2')||section.querySelector('h3'))?.textContent||'';}
  function codeEntry(path,chip,row,summary){
    var key=path+'|'+chip.textContent,entry=codeEntries.get(key);
    if(!entry){entry={title:chip.textContent,summary:summary||'',path:path,component:'',section:'',sections:[],kind:'code',type:rmT('Code'),source:chip,memberships:[],destination:row};codeEntries.set(key,entry);add(entry);}
    if(!entry.summary&&summary)entry.summary=summary;
    if(entry.source.tagName!=='A'&&chip.tagName==='A')entry.source=chip;
    return entry;
  }
  function belongs(entry,section,membership){
    if(!entry.sections.includes(section.id))entry.sections.push(section.id);
    if(!entry.memberships.some(function(m){return m.program===membership.program&&(m.node||m.destination)===(membership.node||membership.destination);}))entry.memberships.push(membership);
  }
  document.querySelectorAll('.group .inventory-file').forEach(function(file){
    var group=file.closest('.group'),node=groupNodes[group.id],section=group.closest('section'),path=file.querySelector('summary').textContent.split(' · ')[0];
    file.querySelectorAll('.symbol-index li').forEach(function(row){
      var chip=row.querySelector('.chip');if(!chip)return;
      var entry=codeEntry(path,chip,row,modelText(row.querySelector(':scope>.model')));
      if(node)belongs(entry,section,{node:node,map:node.closest('[data-map-explorer]'),program:programOf(section),title:programOf(section)+' / '+node.dataset.title});
      else if(!entry.sections.includes(section.id))entry.sections.push(section.id);
    });
  });
  // A declaration no part holds, such as one no box of its split file took
  // or one of a part its program never runs, is still code to find: it
  // opens its row and its source, and no part.
  document.querySelectorAll('.off-map-catalog [data-off-map-file] .chip, .unreached-parts [data-off-map-file] .chip').forEach(function(chip){
    var row=chip.closest('[data-off-map-file]'),section=chip.closest('[data-report-page]');if(!section)return;
    var entry=codeEntry(row.dataset.path,chip,row,'');
    var list=chip.closest('.unreached-parts')?rmT('Not reachable from the entrypoints'):rmT('Not on the map');
    belongs(entry,section,{destination:row,program:programOf(section),title:programOf(section)+' / '+list});
  });
  codeEntries.forEach(function(entry){
    entry.memberships.sort(function(a,b){return a.program.localeCompare(b.program,document.documentElement.lang);});
    entry.component=Array.from(new Set(entry.memberships.map(function(m){return m.program;}))).join(', ')||entry.sections.map(function(id){return components[id]||'';}).join(', ');
    if(entry.sections.length===1)entry.section=entry.sections[0];
    entry.haystack=(entry.title+' '+entry.summary+' '+entry.path+' '+entry.component).toLowerCase();
  });
  // A code result shows its declaration the way its tile does: a function
  // with what it takes and returns, a type with its fields.
  var symbolLists=new WeakMap();
  function tileRows(entry){
    var href=entry.source.getAttribute('href');
    for(var i=0;href&&i<entry.memberships.length;i++){
      var node=entry.memberships[i].node;if(!node)continue;
      if(!symbolLists.has(node)){try{symbolLists.set(node,JSON.parse(node.dataset.symbols||'[]'));}catch(_){symbolLists.set(node,[]);}}
      var symbols=symbolLists.get(node),at=symbols.findIndex(function(symbol){return symbol.href===href&&symbol.kind!=='field';});
      if(at<0)continue;
      return {head:symbols[at].name+(symbols[at].text||''),fields:symbols.filter(function(symbol){return symbol.owner===at+1&&(symbol.kind==='field'||symbol.kind==='more');}).map(function(symbol){return symbol.name+(symbol.text||'');})};
    }
    var fields=entry.destination?Array.from(entry.destination.querySelectorAll(':scope>.symbol-fields>li>.chip')).map(function(chip){return chip.firstChild.textContent;}):[];
    return {head:entry.title.replace(/:\d+$/,''),fields:fields};
  }
  function sectionsFor(node,selector){
    return Array.from(new Set(Array.from(node.querySelectorAll(selector)).map(function(a){
      var n=document.getElementById(a.dataset.questionMap||(a.getAttribute('href')||'').slice(1));
      return n?.dataset.owner||n?.closest('section')?.id;
    }).filter(Boolean)));
  }
  document.querySelectorAll('.reading-guide').forEach(function(n){
    var summary=Array.from(n.querySelectorAll('.answer-prose')).map(function(p){return p.textContent;}).join('\n\n');
    add({title:n.querySelector('.reading-question').textContent,summary:summary,component:rmT('Repository questions'),section:'questions',sections:sectionsFor(n,'[data-question-map]'),kind:'question',type:rmT('Question'),destination:n});
  });
  document.querySelectorAll('.learn-concept').forEach(function(n){
    add({title:n.querySelector('summary').textContent,summary:modelText(n.querySelector('.model')),component:rmT('Term explanation'),section:'concepts',sections:sectionsFor(n,'a[href^="#"]'),kind:'term',type:rmT('Term'),destination:n});
  });
  function expanded(value){box.setAttribute('aria-expanded',value);}
  function changed(){box.dispatchEvent(new CustomEvent('repomap:find',{bubbles:true}));}
  var lastPlace='',lastScroll=0;
  function close(record){if(!panel.hidden)lastScroll=results.scrollTop;panel.hidden=true;expanded('false');if(record!==false)changed();}
  // Choosing a component on the map closes the results and searches that
  // component next: the two choices no longer disagree about where one is.
  box.chooseComponent=function(id){component.value=Array.from(component.options).some(function(o){return o.value===id;})?id:'';page=0;close();};
  box.searchState=function(){return {query:box.value,kind:kind.value,component:component.value,page:page,open:!panel.hidden};};
  box.restoreSearch=function(saved){
    saved=saved||{};box.value=typeof saved.query==='string'?saved.query:'';
    kind.value=Array.from(kind.options).some(function(o){return o.value===saved.kind;})?saved.kind:'all';
    component.value=Array.from(component.options).some(function(o){return o.value===saved.component;})?saved.component:'';
    lastQuery=box.value.trim().toLowerCase();page=Number.isInteger(saved.page)&&saved.page>=0?saved.page:0;
    render();if(!saved.open)close();
  };
  function dismiss(){close();box.focusSearch();close();}
  async function go(entry){
    // Keep the result list in the previous history entry when opening a hit.
    close(false);
    if(entry.node&&entry.map?.findNode){await entry.map.findNode(entry.node,entry.codeSource);return;}
    document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:entry.node||entry.destination}}));
    var destination=entry.destination;
    if(destination){for(var parent=destination.parentElement;parent;parent=parent.parentElement)if(parent.tagName==='DETAILS')parent.open=true;destination.scrollIntoView({block:'start'});}
  }
  function action(label,entry){var button=document.createElement('button');button.type='button';button.textContent=label;button.addEventListener('click',function(){go(entry);});return button;}
  function membership(entry,m){return m.node?Object.assign({},m,{codeSource:{href:entry.source.getAttribute('href'),open:entry.source.dataset.open}}):{destination:m.destination};}
  function appendText(parent,tag,text,cls){var el=document.createElement(tag);el.textContent=text;if(cls)el.className=cls;parent.appendChild(el);return el;}
  function rank(e,q){var title=e.title.toLowerCase();if(e.kind==='code')title=title.replace(/:\d+$/,'');return (title===q?0:title.startsWith(q)?1:title.includes(q)?2:3)*10+(e.kind==='code'?1:0);}
  function description(entry,li,terms){
    // The full text remains indexed and in its original reading. A result is
    // only a locator; it does not open a third layer of citation popovers.
    var summary=entry.summary.replace(/\s+/g,' '),at=summary.toLowerCase().indexOf(terms[0]);
    var start=Math.max(0,at-32),end=Math.min(summary.length,start+140);
    if(start)start=summary.lastIndexOf(' ',start)+1;
    if(end<summary.length){var space=summary.lastIndexOf(' ',end);if(space>start)end=space;}
    appendText(li,'p',(start?'… ':'')+summary.slice(start,end)+(end<summary.length?' …':''),'find-result-summary');
  }
  function render(){
    var q=box.value.trim().toLowerCase();if(q!==lastQuery){page=0;lastQuery=q;}
    if(!q){close();return;}
    var terms=q.split(/\s+/),inScope=entries.filter(function(e){return (!component.value||e.section===component.value||e.sections?.includes(component.value))&&terms.every(function(t){return e.haystack.includes(t);});});
    var matches=inScope.filter(function(e){return kind.value==='all'||e.kind===kind.value;});
    matches.sort(function(a,b){return rank(a,q)-rank(b,q)||a.title.localeCompare(b.title,document.documentElement.lang)||a.component.localeCompare(b.component,document.documentElement.lang);});
    page=Math.min(page,Math.max(0,Math.ceil(matches.length/pageSize)-1));
    var wasOpen=!panel.hidden,scrolled=results.scrollTop;
    panel.hidden=false;expanded('true');results.replaceChildren();
    status.textContent=matches.length?rmT('{0} results',matches.length):kind.value!=='all'?rmT('No matches in {0} with the current filters.',kind.selectedOptions[0].textContent):rmT('No matches with the current filters. Try a shorter name or another word.');
    if(!matches.length&&inScope.length){
      var other=document.createElement('button');other.type='button';other.textContent=rmT('Show other matches ({0})',inScope.length);
      other.addEventListener('click',function(){kind.value='all';page=0;render();kind.focus();});
      status.append(document.createTextNode(' '),other);
    }
    matches.slice(page*pageSize,(page+1)*pageSize).forEach(function(e){
      var li=document.createElement('li'),head=document.createElement('div');head.className='find-result-head';
      var target=e.kind==='code'&&e.memberships.length===1?membership(e,e.memberships[0]):e;
      var shown=e.kind==='code'?tileRows(e):null,title=shown?shown.head:e.title;
      // A declaration in several places is chosen by its "In" links; one no
      // map holds still opens its row in the report.
      if(e.kind==='code'&&e.memberships.length>1)appendText(head,'strong',title,'find-code');else{var chosen=action(title,target);if(shown)chosen.classList.add('find-code');head.appendChild(chosen);}
      appendText(head,'span',e.type,'find-result-type');li.appendChild(head);
      if(shown&&shown.fields.length)appendText(li,'p',shown.fields.join(' · '),'find-result-fields');
      appendText(li,'p',e.kind==='code'?e.path+(e.source.querySelector?.('.ln')?.textContent||''):e.component+(e.path?' · '+e.path:''),'find-result-place');
      if(e.summary)description(e,li,terms);
      if(e.kind==='code'){
        var links=document.createElement('div');links.className='find-result-links';
        e.memberships.forEach(function(m){links.appendChild(action(rmT('In {0} →',m.title),membership(e,m)));});
        if(e.source.tagName==='A'){var source=e.source.cloneNode(false);source.className='find-source';source.textContent=rmT('Open code ↗');links.appendChild(source);}
        li.appendChild(links);
      }
      results.appendChild(li);
    });
    // Back to search returns to the same place in the same list.
    var place=q+'\0'+kind.value+'\0'+component.value+'\0'+page;
    results.scrollTop=place===lastPlace?(wasOpen?scrolled:lastScroll):0;lastPlace=place;
    pages.hidden=matches.length<=pageSize;pages.querySelector('span').textContent=rmT('{0}–{1} of {2}',page*pageSize+1,Math.min((page+1)*pageSize,matches.length),matches.length);
    pages.querySelector('[data-prev]').disabled=page===0;pages.querySelector('[data-next]').disabled=(page+1)*pageSize>=matches.length;
    changed();
  }
  box.addEventListener('input',render);box.addEventListener('focus',function(){if(box.value)render();});
  [kind,component].forEach(function(select){select.addEventListener('change',function(){page=0;render();});});
  panel.querySelector('[data-close]').addEventListener('click',dismiss);
  pages.querySelector('[data-prev]').addEventListener('click',function(){page--;render();});pages.querySelector('[data-next]').addEventListener('click',function(){page++;render();});
  function keydown(e){if(e.key==='Escape'){e.preventDefault();e.stopPropagation();dismiss();}if(e.target===box&&(e.key==='Enter'||e.key==='ArrowDown')){var first=results.querySelector('button')||status.querySelector('button');if(first&&!panel.hidden){e.preventDefault();first.focus();}}}
  nav.addEventListener('keydown',keydown);panel.addEventListener('keydown',keydown);
  document.addEventListener('keydown',function(e){if(e.key==='/'&&!e.ctrlKey&&!e.metaKey&&!e.altKey&&!/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)&&!document.activeElement.isContentEditable){e.preventDefault();box.focusSearch().scrollIntoView({block:'center'});}});
  document.addEventListener('click',function(e){var a=e.target.closest('a[data-question-map]');if(!a)return;var node=document.getElementById(a.dataset.questionMap),map=node?.closest('[data-map-explorer]');if(map?.findNode){e.preventDefault();map.findNode(node);}});
})();
