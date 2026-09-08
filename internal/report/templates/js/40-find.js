// Find existing descriptions and source entries, then open their actual map.
// This is a local index of the complete HTML, not another interpretation pass.
(function () {
  var nav=document.querySelector('nav.nav');
  if(!nav)return;
  var box=document.createElement('input');box.type='search';box.className='find';
  box.placeholder=rmT('Find a question, term, part, operation or code');box.setAttribute('aria-label',rmT('Find in repository'));
  var panel=document.createElement('section');panel.className='find-panel';panel.hidden=true;panel.id='repository-find';
  panel.setAttribute('aria-label',rmT('Repository search results'));box.setAttribute('aria-controls',panel.id);
  panel.innerHTML=("<div class=\"find-tools\"><label>"+rmT.html("Show")+" <select data-find-kind aria-label=\""+rmT.html("Search result type")+"\"><option value=\"all\">"+rmT.html("Everything")+"</option><option value=\"question\">"+rmT.html("Questions and answers")+"</option><option value=\"term\">"+rmT.html("Terms")+"</option><option value=\"part\">"+rmT.html("Parts")+"</option><option value=\"operation\">"+rmT.html("Operations")+"</option><option value=\"code\">"+rmT.html("Code")+"</option></select></label><label>"+rmT.html("In")+" <select data-find-component aria-label=\""+rmT.html("Search component")+"\"><option value=\"\">"+rmT.html("All components")+"</option></select></label><button type=\"button\" data-close>"+rmT.html("Close")+"</button></div><p class=\"find-status\" role=\"status\"></p><ol class=\"find-results\"></ol><div class=\"find-pages\"><button type=\"button\" data-prev>"+rmT.html("← Previous")+"</button><span></span><button type=\"button\" data-next>"+rmT.html("Next →")+"</button></div>");
  nav.append(box,panel);
  var proxy=document.querySelector('[data-reading-query]');
  if(proxy)proxy.setAttribute('aria-controls',panel.id);
  // The same finder can be presented in the Work entrance or the toolbar.
  // Focus follows the visible input; its index and results have one owner.
  box.focusSearch=function(){var input=proxy&&proxy.getClientRects().length?proxy:box;input.focus({preventScroll:true});return input;};
  var kind=panel.querySelector('[data-find-kind]'),component=panel.querySelector('[data-find-component]'),status=panel.querySelector('.find-status'),results=panel.querySelector('ol'),pages=panel.querySelector('.find-pages'),page=0,pageSize=12;
  var entries=[],components={},groupNodes={},codeEntries=new Map(),lastQuery='';
  function modelText(node){if(!node)return '';var copy=node.cloneNode(true);copy.querySelectorAll('.source-hint,.model-sources').forEach(function(n){n.remove();});return copy.textContent;}
  function add(entry){entry.title=entry.title||'';entry.summary=entry.summary||'';entry.path=entry.path||'';entry.haystack=(entry.title+' '+entry.summary+' '+entry.path+' '+entry.component+' '+(entry.additionalText||'')).toLowerCase();entries.push(entry);}
  document.querySelectorAll('.repo-map .repo-node[data-node]').forEach(function(n){
    var id=(n.getAttribute('href')||'').slice(1);if(!id)return;
    components[id]=n.dataset.title;
    add({title:n.dataset.title,summary:n.dataset.summary,component:n.dataset.title,section:id,kind:'part',type:rmT('Component'),destination:document.getElementById(id)});
  });
  Object.keys(components).sort(function(a,b){return components[a].localeCompare(components[b],document.documentElement.lang);}).forEach(function(id){var option=document.createElement('option');option.value=id;option.textContent=components[id];component.appendChild(option);});
  document.querySelectorAll('[data-map-explorer] [data-node]').forEach(function(n){
    if(n.dataset.remote==='true')return;
    var map=n.closest('[data-map-explorer]');
    if(map.displayedNode(n)!==n)return;
    var section=n.closest('section'),id=section.id,href=n.getAttribute('href');
    if(href&&href[0]==='#'&&!n.dataset.activation&&!n.dataset.branch)groupNodes[href.slice(1)]=n;
    add({title:n.dataset.title,summary:n.dataset.summary,additionalText:map.areaDescriptions(n).join(' '),component:components[id]||section.querySelector('h2').textContent,section:id,kind:n.dataset.activation?'operation':'part',type:n.dataset.activation?rmT(n.dataset.activation):(n.dataset.branch?rmT('Area'):rmT('Part')),node:n,map:map});
  });
  // Retain every displayed membership; overlapping executable/library views
  // stay separate and explicitly labelled.
  document.querySelectorAll('.group .inventory-file').forEach(function(file){
    var group=file.closest('.group'),node=groupNodes[group.id],section=group.closest('section'),path=file.querySelector('summary').textContent.split(' · ')[0];
    file.querySelectorAll('.symbol-index li').forEach(function(row){
      var chip=row.querySelector('.chip');if(!chip)return;
      var key=section.id+'|'+path+'|'+chip.textContent,entry=codeEntries.get(key);
      if(!entry){entry={title:chip.textContent,summary:modelText(row.querySelector('.model')),path:path,component:components[section.id]||'',section:section.id,kind:'code',type:rmT('Code'),source:chip,sourceCard:document.getElementById(row.querySelector('.source-hint')?.getAttribute('aria-controls')),memberships:[],destination:row};codeEntries.set(key,entry);add(entry);}
      if(node&&!entry.memberships.some(function(m){return m.node===node;}))entry.memberships.push({node:node,map:node.closest('[data-map-explorer]'),title:node.dataset.title});
    });
  });
  function sourceCard(node,selector){
    var card=document.createElement('aside');card.className='source-card';
    appendText(card,'strong',rmT('Model response'));
    var sources=document.createElement('div');sources.className='source-links';
    node.querySelectorAll(selector+' .model-sources').forEach(function(n){var copy=n.cloneNode(true);copy.className='';copy.hidden=false;sources.appendChild(copy);});
    appendText(card,'p',sources.querySelector('a')?rmT('The model cited these sources:'):rmT('No citations were saved for this text.'));
    sources.querySelectorAll('[hidden]').forEach(function(n){n.hidden=false;});card.appendChild(sources);return card;
  }
  function sectionsFor(node,selector){
    return Array.from(new Set(Array.from(node.querySelectorAll(selector)).map(function(a){
      var n=document.getElementById(a.dataset.questionMap||(a.getAttribute('href')||'').slice(1));
      return n?.closest('section')?.id;
    }).filter(Boolean)));
  }
  document.querySelectorAll('.reading-guide').forEach(function(n){
    var summary=Array.from(n.querySelectorAll('.answer-prose')).map(function(p){return p.textContent;}).join('\n\n');
    add({title:n.querySelector('.reading-question').textContent,summary:summary,component:rmT('Repository questions'),section:'questions',sections:sectionsFor(n,'[data-question-map]'),kind:'question',type:rmT('Question'),destination:n,sourceCard:sourceCard(n,'.answer-copy')});
  });
  document.querySelectorAll('.learn-concept').forEach(function(n){
    add({title:n.querySelector('summary').textContent,summary:modelText(n.querySelector('.model')),component:rmT('Term explanation'),section:'concepts',sections:sectionsFor(n,'a[href^="#"]'),kind:'term',type:rmT('Term'),destination:n,sourceCard:sourceCard(n,':scope')});
  });
  function expanded(value){box.setAttribute('aria-expanded',value);if(proxy)proxy.setAttribute('aria-expanded',value);}
  function changed(){box.dispatchEvent(new CustomEvent('repomap:find',{bubbles:true}));}
  function close(record){panel.hidden=true;expanded('false');if(record!==false)changed();}
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
    document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:entry.node||entry.destination}}));
    if(entry.node&&entry.map?.findNode){await entry.map.findNode(entry.node);if(entry.codeSource)entry.map.explainSource(entry.codeSource);return;}
    var destination=entry.destination;
    if(destination){for(var parent=destination.parentElement;parent;parent=parent.parentElement)if(parent.tagName==='DETAILS')parent.open=true;destination.scrollIntoView({block:'start'});}
  }
  function action(label,entry){var button=document.createElement('button');button.type='button';button.textContent=label;button.addEventListener('click',function(){go(entry);});return button;}
  function membership(entry,m){return Object.assign({},m,{codeSource:{href:entry.source.getAttribute('href'),open:entry.source.dataset.open}});}
  function appendText(parent,tag,text,cls){var el=document.createElement(tag);el.textContent=text;if(cls)el.className=cls;parent.appendChild(el);return el;}
  function rank(e,q){var title=e.title.toLowerCase(),base=e.kind==='code'?10:0;if(e.kind==='code')title=title.replace(/:\d+$/,'');return base+(title===q?0:title.startsWith(q)?1:title.includes(q)?2:3);}
  function description(entry,li,terms){
    var summary=entry.summary;
    if(entry.kind==='question'){
      // Search all answer prose; the preview only locates the matching passage.
      // Opening the result reads the complete original answer, including gaps.
      var at=summary.toLowerCase().indexOf(terms[0]),start=Math.max(0,at-80),end=start+320;
      if(start)start=summary.lastIndexOf(' ',start)+1;
      if(end<summary.length){var space=summary.indexOf(' ',end);if(space!==-1)end=space;}
      summary=(start?'… ':'')+summary.slice(start,end)+(end<summary.length?' …':'');
    }
    var text=appendText(li,'p',summary,'find-result-summary model-inspectable');
    if(!entry.sourceCard){var card=document.createElement('aside');card.className='source-card';appendText(card,'strong',rmT('Model response'));appendText(card,'p',rmT('This is the description used on the map. No separate citations were saved for this text.'));entry.sourceCard=card;}
    var hint=document.createElement('button');hint.type='button';hint.className='source-hint';hint.textContent='ⓘ';hint.setAttribute('aria-label',rmT('About this description'));text.appendChild(hint);repomapPreview.bind(text,entry.sourceCard);
  }
  function render(){
    if(proxy)proxy.value=box.value;
    var q=box.value.trim().toLowerCase();if(q!==lastQuery){page=0;lastQuery=q;}
    if(!q){close();return;}
    var terms=q.split(/\s+/),inScope=entries.filter(function(e){return (!component.value||e.section===component.value||e.sections?.includes(component.value))&&terms.every(function(t){return e.haystack.includes(t);});});
    var matches=inScope.filter(function(e){return kind.value==='all'||e.kind===kind.value;});
    matches.sort(function(a,b){return rank(a,q)-rank(b,q)||a.title.localeCompare(b.title,document.documentElement.lang)||a.component.localeCompare(b.component,document.documentElement.lang);});
    page=Math.min(page,Math.max(0,Math.ceil(matches.length/pageSize)-1));
    panel.hidden=false;expanded('true');results.replaceChildren();
    status.textContent=matches.length?rmT('{0} results · choose one to open its answer, explanation, map or source',matches.length):kind.value!=='all'?rmT('No matches in {0} with the current filters.',kind.selectedOptions[0].textContent):rmT('No matches with the current filters. Try a shorter name or another word.');
    if(!matches.length&&inScope.length){
      var other=document.createElement('button');other.type='button';other.textContent=rmT('Show other matches ({0})',inScope.length);
      other.addEventListener('click',function(){kind.value='all';page=0;render();kind.focus();});
      status.append(document.createTextNode(' '),other);
    }
    matches.slice(page*pageSize,(page+1)*pageSize).forEach(function(e){
      var li=document.createElement('li'),head=document.createElement('div');head.className='find-result-head';
      var target=e.kind==='code'&&e.memberships.length===1?membership(e,e.memberships[0]):e;
      if(e.kind==='code'&&e.memberships.length>1)appendText(head,'strong',e.title);else head.appendChild(action(e.title,target));
      appendText(head,'span',e.type,'find-result-type');li.appendChild(head);
      appendText(li,'p',e.component+(e.path?' · '+e.path:''),'find-result-place');
      if(e.summary)description(e,li,terms);
      if(e.kind==='code'){
        var links=document.createElement('div');links.className='find-result-links';
        e.memberships.forEach(function(m){links.appendChild(action(rmT('In {0} →',m.title),membership(e,m)));});
        if(e.source.tagName==='A'){var source=e.source.cloneNode(false);source.className='find-source';source.textContent=rmT('Open code ↗');links.appendChild(source);}
        li.appendChild(links);
      }
      results.appendChild(li);
    });
    results.scrollTop=0;
    pages.hidden=matches.length<=pageSize;pages.querySelector('span').textContent=rmT('{0}–{1} of {2}',page*pageSize+1,Math.min((page+1)*pageSize,matches.length),matches.length);
    pages.querySelector('[data-prev]').disabled=page===0;pages.querySelector('[data-next]').disabled=(page+1)*pageSize>=matches.length;
    changed();
  }
  box.addEventListener('input',render);box.addEventListener('focus',function(){if(box.value)render();});
  [kind,component].forEach(function(select){select.addEventListener('change',function(){page=0;render();});});
  panel.querySelector('[data-close]').addEventListener('click',dismiss);
  pages.querySelector('[data-prev]').addEventListener('click',function(){page--;render();});pages.querySelector('[data-next]').addEventListener('click',function(){page++;render();});
  function keydown(e){if(e.key==='Escape'){e.preventDefault();e.stopPropagation();dismiss();}if((e.target===box||e.target===proxy)&&(e.key==='Enter'||e.key==='ArrowDown')){var first=results.querySelector('button')||status.querySelector('button');if(first&&!panel.hidden){e.preventDefault();first.focus();}}}
  nav.addEventListener('keydown',keydown);panel.addEventListener('keydown',keydown);
  if(proxy){
    proxy.addEventListener('input',function(){box.value=proxy.value;box.dispatchEvent(new Event('input',{bubbles:true}));});
    proxy.addEventListener('focus',function(){if(box.value)render();});
    proxy.addEventListener('keydown',keydown);
  }
  document.addEventListener('keydown',function(e){if(e.key==='/'&&!e.ctrlKey&&!e.metaKey&&!e.altKey&&!/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement.tagName)&&!document.activeElement.isContentEditable){e.preventDefault();box.focusSearch().scrollIntoView({block:'center'});}});
  document.addEventListener('click',function(e){var a=e.target.closest('a[data-question-map]');if(!a)return;var node=document.getElementById(a.dataset.questionMap),map=node?.closest('[data-map-explorer]');if(map?.findNode){e.preventDefault();document.dispatchEvent(new CustomEvent('repomap:navigate',{detail:{destination:node}}));map.findNode(node);}});
})();
