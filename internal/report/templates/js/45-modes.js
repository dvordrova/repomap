// Two entrances, one report. Changing entrance preserves the selected place;
// local reading actions and browser Back retain the preceding reading path.
(function(){
  var body=document.body, bar=document.querySelector('.reading-modes');
  var pages=Array.from(document.querySelectorAll('main > [data-report-page]'));
  var home=document.getElementById('overview'), repoMap=document.getElementById('repository-map');
  if(!bar||!home)return;
  var toolbar=bar.closest('.report-toolbar');
  var locationLine=toolbar.querySelector('.reading-location'),locationName=document.createElement('span'),mapContext=document.createElement('a'),readingIntent=document.createElement('span');
  readingIntent.className='reading-intent';readingIntent.hidden=true;locationName.className='reading-address';
  mapContext.className='reading-map-context';mapContext.dataset.readingMapReturn='';mapContext.hidden=true;locationLine.append(readingIntent,locationName,mapContext);
  var returnLinks=document.createElement('nav');returnLinks.className='reading-return';returnLinks.hidden=true;locationLine.appendChild(returnLinks);
  var returnLink=document.createElement('a'),termLink=document.createElement('a'),mapLink=document.createElement('a'),searchLink=document.createElement('button');mapLink.dataset.readingMapReturn='';searchLink.type='button';searchLink.dataset.readingFind='';returnLinks.append(returnLink,termLink,mapLink,searchLink);
  var detailGroup=null;
  function measureToolbar(){document.documentElement.style.setProperty('--toolbar-height',getComputedStyle(toolbar).position==='sticky'?toolbar.getBoundingClientRect().height+'px':'0px');}
  new ResizeObserver(measureToolbar).observe(toolbar);measureToolbar();
  var current=home, mode='learn', question=null, term=null, searchIntent='',restoring=false;
  var globalSearch=document.querySelector('.nav .find'),workSearch=document.querySelector('[data-reading-query]');
  var questionPage=document.getElementById('questions'),guides=questionPage?Array.from(questionPage.querySelectorAll('.reading-guide')):[];
  var questionIndex=questionPage&&questionPage.querySelector('.question-index');
  function enclosing(node){return node&&node.closest('[data-report-page]');}
  function locate(){try{return document.getElementById(decodeURIComponent(location.hash.slice(1)));}catch(e){return null;}}
  // A map may be reached from several questions. Its URL names the place;
  // this browser history entry retains why that particular visit was opened.
  function readingState(){
    var map=current.querySelector('[data-map-explorer]');
    return Object.assign({},history.state,{repomapReading:{question:question?.id||'',term:term?.id||'',search:searchIntent,find:globalSearch?.searchState(),map:map?.readingState?{page:current.id,value:map.readingState()}:null}});
  }
  function remember(){if(!restoring&&!current.querySelector('[data-map-explorer]')?.readingRestoring)history.replaceState(readingState(),'',location.href);}
  function savedElement(id,selector){var node=typeof id==='string'&&document.getElementById(id);return node&&node.matches(selector)?node:null;}
  function selectQuestion(selected){
    guides.forEach(function(guide){guide.hidden=guide!==selected;guide.open=guide===selected;});
    if(questionPage)questionPage.classList.toggle('has-selected-question',!!selected);
    if(questionIndex)questionIndex.open=!selected;
    document.querySelectorAll('.question-menu a').forEach(function(a){
      if(selected&&a.getAttribute('href')==='#'+selected.id)a.setAttribute('aria-current','true');else a.removeAttribute('aria-current');
    });
  }
  function textElement(tag,text,className){var el=document.createElement(tag);el.textContent=text;if(className)el.className=className;return el;}
  function questionLink(guide){var a=textElement('a',guide.querySelector('.reading-question').textContent);a.href='#'+guide.id;return a;}
  function prepareReadingEntrances(){
    guides.forEach(function(guide,index){
      var position=document.createElement('nav');position.className='question-position';position.setAttribute('aria-label',rmT('Question navigation'));
      position.appendChild(textElement('span',rmT('Question {0} of {1}',index+1,guides.length)));
      var all=textElement('a',rmT('← All questions'));all.href='#questions';position.appendChild(all);
      if(index+1<guides.length){var next=textElement('a',rmT('Next question →'));next.href='#'+guides[index+1].id;position.appendChild(next);}
      guide.querySelector('.reading-question').after(position);

    });
    pages.filter(function(page){return page.hasAttribute('data-component-name');}).forEach(function(page){
      var map=page.querySelector('[data-map-explorer]');if(!map)return;
      // Only the answer's existing exact map links establish this menu. It
      // does not guess relevance from names, proximity or the selected node.
      var linked=guides.filter(function(guide){return Array.from(guide.querySelectorAll('[data-question-map]')).some(function(a){
        var node=document.getElementById(a.dataset.questionMap||(a.getAttribute('href')||'').slice(1));return enclosing(node)===page;
      });});
      var learn=document.createElement('aside');learn.className='component-reading-entry';learn.dataset.readingMode='learn';
      if(linked.length){
        var menu=document.createElement('details');menu.className='component-question-menu';
        menu.appendChild(textElement('summary',rmT('{0} questions link here',linked.length)));
        var list=document.createElement('ol');list.className='question-menu';linked.forEach(function(guide){var li=document.createElement('li');li.appendChild(questionLink(guide));list.appendChild(li);});menu.appendChild(list);learn.appendChild(menu);
      }else learn.appendChild(textElement('span',rmT('No saved question links to this component.')));
      var all=textElement('a',rmT('All questions'));all.href='#questions';if(guides.length)learn.appendChild(all);
      var work=document.createElement('aside');work.className='component-reading-entry';work.dataset.readingMode='work';
      var button=textElement('button',rmT('Find in repository'),'reading-search-entry');button.type='button';button.dataset.readingFind='';
      var component=document.querySelector('[data-find-component]');
      if(component&&Array.from(component.options).some(function(option){return option.value===page.id;})){button.dataset.readingFind=page.id;button.textContent=rmT('Find in this component');}
      // Map navigation scrolls to the figure itself. Keep the reading
      // entrance inside that figure so the sticky toolbar cannot cover it.
      work.appendChild(button);map.prepend(learn,work);
    });
    document.querySelectorAll('[data-reading-find]').forEach(function(button){button.hidden=false;});
  }
  function showReturn(){
    returnLink.hidden=!question||current===enclosing(question);
    if(question){returnLink.href='#'+question.id;returnLink.textContent=rmT('Back to question');returnLink.title=question.querySelector('.reading-question').textContent;}
    termLink.hidden=!!question||!term||current===enclosing(term);
    if(term){termLink.href='#'+term.id;termLink.textContent=rmT('Back to term');termLink.title=term.querySelector('summary').textContent;}
    var map=detailGroup&&current.querySelector('[data-map-explorer]');
    mapLink.hidden=!map;
    if(map){mapLink.href='#'+mapDestination(map).id;mapLink.textContent=rmT('Back to map');mapLink.title=map.explorationLabel();}
    searchLink.hidden=!searchIntent||!!question;searchLink.textContent=rmT('Back to search');
    returnLinks.hidden=returnLink.hidden&&termLink.hidden&&mapLink.hidden&&searchLink.hidden;
  }
  function showLocation(){
    var repositorySelection=current===home&&home.querySelector('.repo-map')?.dataset.readingLabel;
    var place=current===home?(repositorySelection||rmT('Home')):(current.dataset.componentName||current.querySelector('h2')?.textContent||'');
    if(current===questionPage&&question)place=rmT('Question {0} of {1}',guides.indexOf(question)+1,guides.length);
    else if(term&&current===enclosing(term))place=term.querySelector('summary').textContent;
    var detailTitle=detailGroup&&detailGroup.querySelector('.group-head h4').cloneNode(true);
    if(detailTitle)detailTitle.querySelectorAll('button,.model-sources').forEach(function(n){n.remove();});
    locationName.textContent=place+(detailTitle?' · '+rmT('Full details: {0}',detailTitle.textContent):'');
    readingIntent.textContent=question?question.querySelector('.reading-question').textContent:searchIntent?rmT('Search: {0}',searchIntent):'';
    readingIntent.hidden=!readingIntent.textContent;
    var map=!detailGroup&&current.querySelector('[data-map-explorer]');
    mapContext.hidden=!map;
    if(map){mapContext.href='#'+mapDestination(map).id;mapContext.textContent=map.explorationLabel();}
  }
  function mapDestination(map){return document.getElementById(map.explorerScope)||map.explorerOperation||current;}
  function placeSearch(){
    var panel=document.querySelector('.find-panel'),container=current===home&&mode==='work'?home.querySelector('.work-intro'):toolbar.querySelector('.nav');
    // The existing finder owns this one result panel and all of its state.
    // Only its presentation moves into the Work entrance while at home.
    if(panel&&container&&panel.parentElement!==container)container.appendChild(panel);
  }
  function setPage(node){
    var page=enclosing(node);if(!page)return;
    var nextQuestion=node.closest('.reading-guide');
    if(page===questionPage)selectQuestion(nextQuestion);
    if(nextQuestion){question=nextQuestion;term=null;searchIntent='';}
    else if(page===home||node.id==='questions'){question=null;term=null;searchIntent='';}
    var nextTerm=node.closest('.learn-concept');
    if(nextTerm)term=nextTerm;
    else if(node.id==='concepts')term=null;
    revealConcept(node);
    current=page;body.dataset.readingPage=page.id;detailGroup=node.closest('.group');pages.forEach(function(p){p.hidden=p!==page;});
    placeSearch();showReturn();showLocation();
    for(var p=node;p&&p!==page;p=p.parentElement)if(p.tagName==='DETAILS')p.open=true;
    document.querySelectorAll('.target-picker').forEach(function(p){p.open=false;});
    // Navigation scrolls immediately; the return links and search panel may
    // have changed this height before ResizeObserver gets its next turn.
    measureToolbar();
  }
  function address(node,replace){
    if(!node.id)node=enclosing(node);
    var url=new URL(location.href);url.searchParams.set('mode',mode);url.hash=node.id;
    if(url.href!==location.href)history[replace?'replaceState':'pushState'](readingState(),'',url);
  }
  function setMode(next,record){
    mode=next==='work'?'work':'learn';body.dataset.readingMode=mode;
    document.querySelectorAll('[data-reading-mode]').forEach(function(el){if(el!==body)el.hidden=el.dataset.readingMode!==mode;});
    bar.querySelectorAll('[data-mode]').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.mode===mode);});
    placeSearch();
    // A single-component report has no repository SVG; its existing card
    // remains a direct entrance instead of an empty Work home.
    if(!repoMap.querySelector('.repo-map'))repoMap.querySelector('.evidence-list').open=true;
    if(record){var url=new URL(location.href);url.searchParams.set('mode',mode);history.pushState(readingState(),'',url);}
  }
  // All navigators (search, map deep links and ordinary anchors) reveal the
  // existing section before any map measures its available space.
  document.addEventListener('repomap:navigate',function(e){var node=e.detail.destination;if(!enclosing(node))return;setPage(node);address(node);});
  // Layout commits the explorer's operation and scope. Hidden maps and the
  // inspector's temporary hover subject must not replace this reading context.
  document.addEventListener('repomap:layout',function(e){if(!detailGroup&&enclosing(e.target)===current)showLocation();},true);
  document.addEventListener('repomap:reading',function(e){if(!detailGroup&&enclosing(e.target)===current){showLocation();remember();}},true);
  document.addEventListener('repomap:viewport',function(e){if(enclosing(e.target)===current)remember();},true);
  document.addEventListener('click',function(e){
    var findAction=e.target.closest('[data-reading-find]');
    if(findAction){
      var search=document.querySelector('.nav .find'),component=document.querySelector('[data-find-component]');
      if(component&&findAction!==searchLink){component.value=findAction.dataset.readingFind||'';component.dispatchEvent(new Event('change',{bubbles:true}));}
      if(search)search.focusSearch();return;
    }
    if(e.target.closest('a')===mapContext&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey&&!e.altKey){
      e.preventDefault();var contextMap=current.querySelector('[data-map-explorer]');contextMap.resumeExploration();contextMap.scrollIntoView({block:'start'});return;
    }
    if(e.target.closest('a')===mapLink&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey&&!e.altKey){
      e.preventDefault();var map=current.querySelector('[data-map-explorer]');
      setPage(map);address(mapDestination(map));map.resumeExploration();map.scrollIntoView({block:'start'});return;
    }
    var readingQuestion=e.target.closest('.reading-guide');
    if(readingQuestion){question=readingQuestion;term=null;searchIntent='';showReturn();showLocation();}
    var readingTerm=e.target.closest('.learn-concept'),termAction=e.target.closest('summary,a');
    if(readingTerm&&termAction&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey&&!e.altKey){
      // A filtered dictionary card can be selected without following its own
      // anchor. Record that exact owner before a source or map link leaves it;
      // browser Back must also reopen this card, not the previous URL's term.
      term=readingTerm;showReturn();showLocation();address(readingTerm,true);
    }
    var a=e.target.closest('a[href^="#"]');if(!a||a.closest('[data-map-explorer]')||a.hasAttribute('data-open')||a.hasAttribute('data-question-map')||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;
    var node=document.getElementById(a.getAttribute('href').slice(1));if(!enclosing(node))return;
    e.preventDefault();setPage(node);address(node);
    // Map-node links are handled by their existing explorer. Scrolling an
    // SVG node directly would pan the page to an offscreen graph coordinate.
    if(!node.matches('[data-node]'))node.scrollIntoView({block:'start'});
  },true);
  bar.querySelectorAll('[data-mode]').forEach(function(b){b.addEventListener('click',function(){
    remember();setMode(b.dataset.mode,true);
  });});
  function restore(){
    restoring=true;
    var saved=history.state?.repomapReading,node=locate()||home;
    question=null;term=null;searchIntent='';setMode(new URL(location.href).searchParams.get('mode'),false);setPage(node);
    if(saved){
      if(!node.closest('.reading-guide'))question=savedElement(saved.question,'.reading-guide');
      term=savedElement(saved.term,'.learn-concept');searchIntent=question?'':typeof saved.search==='string'?saved.search:'';
    }
    globalSearch?.restoreSearch(saved?.find);
    if(saved?.map?.page===current.id)current.querySelector('[data-map-explorer]')?.restoreReadingState(saved.map.value);
    showReturn();showLocation();restoring=false;
  }
  // Restore the visit before a map reacts to the new URL or emits a reading
  // event; otherwise that event could save the visit we are leaving over it.
  window.addEventListener('popstate',restore,true);window.addEventListener('hashchange',restore,true);
  prepareReadingEntrances();bar.hidden=false;body.classList.add('reading-ready');selectQuestion(null);restore();remember();
  var initialHash=location.hash;window.addEventListener('load',function(){document.fonts.ready.then(function(){if(initialHash&&location.hash===initialHash)rmScrollToReading(locate());});},{once:true});
  document.addEventListener('repomap:find',remember);
  if(globalSearch){
    globalSearch.addEventListener('input',function(){
      searchIntent=globalSearch.value.trim();if(searchIntent){question=null;term=null;}
      if(workSearch)workSearch.value=globalSearch.value;showReturn();showLocation();remember();
    });
    if(workSearch){
      workSearch.closest('.work-search').hidden=false;workSearch.value=globalSearch.value;
    }
  }
  document.querySelectorAll('.learn-band').forEach(function(band){band.addEventListener('toggle',function(){if(!band.open)return;document.querySelectorAll('.learn-band').forEach(function(other){if(other!==band)other.open=false;});});});

  var library=document.getElementById('concepts');if(!library)return;
  var search=library.querySelector('input'), entries=Array.from(library.querySelectorAll('.learn-concept'));
  var pager=library.querySelector('.concept-pages'), count=library.querySelector('.concept-count'), page=0, size=8;
  var text=entries.map(function(entry){return entry.textContent.toLowerCase();});
  function revealConcept(node){
    if(!library||!entries||!node)return;
    var entry=node.closest('.learn-concept'),index=entries.indexOf(entry);
    if(index<0)return;
    // A source-linked term may be outside the current search/page. Reveal
    // that exact entry before the common navigator scrolls to its anchor.
    search.value='';page=Math.floor(index/size);filter();entry.open=true;
  }
  function filter(){
    var terms=search.value.toLowerCase().trim().split(/\s+/), found=entries.filter(function(e,i){return terms.every(function(t){return text[i].includes(t);});});
    page=Math.min(page,Math.max(0,Math.ceil(found.length/size)-1));
    var visible=new Set(found.slice(page*size,(page+1)*size));entries.forEach(function(e){e.hidden=!visible.has(e);});
    count.textContent=found.length?rmT('{0} terms',found.length):rmT('No terms match. Try another word from its name or description.');
    pager.hidden=found.length<=size;pager.querySelector('span').textContent=rmT('{0}–{1} of {2}',found.length?page*size+1:0,Math.min((page+1)*size,found.length),found.length);
    pager.querySelector('[data-prev]').disabled=page===0;pager.querySelector('[data-next]').disabled=(page+1)*size>=found.length;
  }
  library.querySelector('.concept-search').hidden=false;
  search.addEventListener('input',function(){page=0;filter();});
  pager.querySelector('[data-prev]').addEventListener('click',function(){page--;filter();library.scrollIntoView({block:'start'});});
  pager.querySelector('[data-next]').addEventListener('click',function(){page++;filter();library.scrollIntoView({block:'start'});});filter();revealConcept(locate());
})();
